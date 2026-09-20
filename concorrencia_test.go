package main

// =====================================================================
// TESTE DE CONCORRÊNCIA DO ENDPOINT POST /movimentacao
//
// Local: estoque-ia/concorrencia_test.go (ao lado do main.go)
//
// Como rodar (PowerShell, dentro da pasta estoque-ia):
//   go test -v -run "TestEstoqueNaoFicaNegativo/estoque_pequeno" -count=5
//
// Para rodar os três cenários:
//   go test -v -run TestEstoqueNaoFicaNegativo -count=20
//
// O -count repete a execução. Condição de corrida é sorteio: pode
// passar numa rodada e falhar na seguinte.
//
// ---------------------------------------------------------------------
// NÃO É PRECISO MEXER NO main.go
//
// O teste monta o próprio roteador (função rotas(), no fim do arquivo),
// registrando os handlers SEM o handlers.Protegido. O alvo aqui é a
// lógica de estoque, não a autenticação — e o /movimentacao de produção
// continua protegido normalmente.
//
// ---------------------------------------------------------------------
// O QUE ESTE TESTE FAZ
//
//   Para cada cenário da tabela abaixo:
//     - cria um banco SQLite novo e descartável em t.TempDir()
//       (o seu estoque.db NÃO é tocado)
//     - injeta esse banco nos handlers via handlers.Iniciar()
//     - cadastra um produto e dá entrada de N unidades
//     - dispara vários clientes ao mesmo tempo, sendo um deles um
//       "atacadista" que pede uma quantidade grande
//     - confere se a soma das saídas aprovadas passou do estoque
//
//   O teste NÃO tenta adivinhar quem ganha a corrida. Ora o atacadista
//   leva tudo, ora os pequenos entram primeiro — as duas coisas são
//   aceitáveis. O que nunca pode acontecer é aprovar mais unidades do
//   que havia em estoque.
//
//   FAIL com "VENDEU DEMAIS" significa que o teste FUNCIONOU: ele achou
//   a condição de corrida que existe hoje no handler.
// =====================================================================

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"estoque-ia/handlers"
	"estoque-ia/models"
)

// ---------------------------------------------------------------------
// Cenários
//
// Regra de ouro: a demanda total precisa passar do estoque inicial.
// Se não passar, ninguém é recusado, não há disputa, e o teste passa
// sem provar nada.
//
// Quanto maior o estoque, mais clientes simultâneos são necessários
// para a janela de corrida aparecer — por isso o cenário grande tem
// 40 clientes, e não 6.
// ---------------------------------------------------------------------

type cenario struct {
	nome            string
	saldoInicial    int
	clientesNormais int
	qtdNormal       int
	qtdAtacadista   int
}

var cenarios = []cenario{
	{nome: "estoque pequeno", saldoInicial: 10, clientesNormais: 6, qtdNormal: 2, qtdAtacadista: 15},
	{nome: "estoque medio", saldoInicial: 25, clientesNormais: 10, qtdNormal: 3, qtdAtacadista: 20},
	{nome: "estoque grande", saldoInicial: 200, clientesNormais: 40, qtdNormal: 6, qtdAtacadista: 120},
}

// Cliente HTTP com pool maior. O http.Post padrão reaproveita só 2
// conexões ociosas por host, o que vira gargalo artificial com muitas
// goroutines e acaba escondendo a corrida.
var clienteHTTP = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
	},
}

func TestEstoqueNaoFicaNegativo(t *testing.T) {
	for _, c := range cenarios {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			// NÃO use t.Parallel(): cada cenário reinjeta a conexão nos
			// handlers, então eles precisam rodar em sequência.

			demanda := c.clientesNormais*c.qtdNormal + c.qtdAtacadista
			if demanda <= c.saldoInicial {
				t.Fatalf("cenário mal montado: demanda total %d não passa do estoque %d — a corrida nunca vai aparecer",
					demanda, c.saldoInicial)
			}

			rodarCenario(t, c)
		})
	}
}

func rodarCenario(t *testing.T, c cenario) {
	t.Helper()

	db := bancoDeTeste(t)

	srv := httptest.NewServer(rotas())
	defer srv.Close()

	produtoID := criarProduto(t, srv.URL, "Toalha tipo A")
	registrarEntrada(t, srv.URL, produtoID, c.saldoInicial)

	type pedido struct {
		cliente    string
		quantidade int
	}
	pedidos := make([]pedido, 0, c.clientesNormais+1)
	for i := 1; i <= c.clientesNormais; i++ {
		pedidos = append(pedidos, pedido{fmt.Sprintf("cliente-%02d", i), c.qtdNormal})
	}
	pedidos = append(pedidos, pedido{"ATACADISTA", c.qtdAtacadista})

	type resultado struct {
		cliente    string
		quantidade int
		status     int
		corpo      string
	}

	// A largada: todas as goroutines ficam bloqueadas esperando este
	// canal fechar. Sem isso elas saem escalonadas, a primeira termina
	// antes da última começar, e a corrida não acontece.
	largada := make(chan struct{})
	resultados := make(chan resultado, len(pedidos))
	var wg sync.WaitGroup

	for _, p := range pedidos {
		wg.Add(1)
		go func(cliente string, qtd int) {
			defer wg.Done()
			<-largada
			status, corpo := enviarSaida(srv.URL, produtoID, qtd)
			resultados <- resultado{cliente, qtd, status, corpo}
		}(p.cliente, p.quantidade)
	}

	close(largada) // tiro de largada: todos saem juntos
	wg.Wait()
	close(resultados)

	aprovados := 0
	unidadesVendidas := 0
	travamentos := 0

	for r := range resultados {
		switch {
		case r.status == http.StatusCreated || r.status == http.StatusOK:
			aprovados++
			unidadesVendidas += r.quantidade
			t.Logf("APROVADO  %-12s pediu %3d", r.cliente, r.quantidade)

		case r.status == http.StatusUnprocessableEntity:
			t.Logf("RECUSADO  %-12s pediu %3d (estoque insuficiente)", r.cliente, r.quantidade)

		case strings.Contains(r.corpo, "database is locked"),
			strings.Contains(r.corpo, "SQLITE_BUSY"):
			travamentos++
			t.Logf("TRAVOU    %-12s pediu %3d (SQLite ocupado)", r.cliente, r.quantidade)

		default:
			t.Errorf("status inesperado %d para %s: %s", r.status, r.cliente, r.corpo)
		}
	}

	t.Logf("---- %d aprovados, %d unidades vendidas, estoque inicial %d",
		aprovados, unidadesVendidas, c.saldoInicial)

	if travamentos > 0 {
		t.Logf("---- atenção: %d requisições falharam por lock do SQLite (problema separado)", travamentos)
	}

	if unidadesVendidas > c.saldoInicial {
		t.Fatalf("VENDEU DEMAIS: aprovou %d unidades tendo apenas %d em estoque (saldo ficaria %d)",
			unidadesVendidas, c.saldoInicial, c.saldoInicial-unidadesVendidas)
	}

	if saldo := saldoNoBanco(t, db, produtoID); saldo < 0 {
		t.Fatalf("SALDO NEGATIVO no banco: %d unidades", saldo)
	}
}

// ---------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------

// bancoDeTeste cria um SQLite novo, vazio e descartável, e o injeta nos
// handlers pelo mesmo caminho que o main.go usa: handlers.Iniciar().
// O arquivo vive em t.TempDir(), que o Go apaga sozinho no fim.
func bancoDeTeste(t *testing.T) *gorm.DB {
	t.Helper()

	caminho := filepath.Join(t.TempDir(), "teste.db")
	dsn := caminho + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrindo banco de teste: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	// AJUSTE se o seu AutoMigrate do database.go incluir outros models
	// (ex: models.Pedido). O banco de teste começa vazio, então toda
	// tabela usada pelos handlers precisa ser criada aqui.
	if err := db.AutoMigrate(&models.Produto{}, &models.Movimentacao{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	handlers.Iniciar(db)

	return db
}

func criarProduto(t *testing.T, baseURL, nome string) uint {
	t.Helper()

	// AJUSTE as chaves do JSON conforme as tags do seu models.Produto
	body := fmt.Sprintf(`{"nome":%q,"preco":49.90}`, nome)

	resp, err := clienteHTTP.Post(baseURL+"/produto", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("criando produto: %v", err)
	}
	defer resp.Body.Close()

	corpo, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("criando produto: status %d, corpo %s", resp.StatusCode, corpo)
	}

	var criado struct {
		ID uint `json:"ID"`
	}
	if err := json.Unmarshal(corpo, &criado); err != nil || criado.ID == 0 {
		// Se o handler não devolve o produto criado, usamos 1: é o
		// primeiro registro de um banco recém-criado.
		return 1
	}
	return criado.ID
}

func registrarEntrada(t *testing.T, baseURL string, produtoID uint, qtd int) {
	t.Helper()

	body := fmt.Sprintf(`{"produto_id":%d,"tipo":"entrada","quantidade":%d}`, produtoID, qtd)

	resp, err := clienteHTTP.Post(baseURL+"/movimentacao", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("entrada inicial: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		corpo, _ := io.ReadAll(resp.Body)
		t.Fatalf("entrada inicial: status %d, corpo %s", resp.StatusCode, corpo)
	}
}

func enviarSaida(baseURL string, produtoID uint, qtd int) (int, string) {
	body := fmt.Sprintf(`{"produto_id":%d,"tipo":"saida","quantidade":%d}`, produtoID, qtd)

	resp, err := clienteHTTP.Post(baseURL+"/movimentacao", "application/json", strings.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()

	corpo, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(corpo)
}

func saldoNoBanco(t *testing.T, db *gorm.DB, produtoID uint) int {
	t.Helper()

	var entradas, saidas int

	db.Model(&models.Movimentacao{}).
		Where("produto_id = ? AND tipo = ?", produtoID, "entrada").
		Select("COALESCE(SUM(quantidade), 0)").
		Scan(&entradas)

	db.Model(&models.Movimentacao{}).
		Where("produto_id = ? AND tipo = ?", produtoID, "saida").
		Select("COALESCE(SUM(quantidade), 0)").
		Scan(&saidas)

	return entradas - saidas
}

// ---------------------------------------------------------------------
// Roteador só do teste.
//
// Registra apenas os endpoints que o teste usa, e SEM o
// handlers.Protegido — o teste não faz login, e autenticação não é o
// que estamos testando aqui. O main.go continua intocado, com as
// rotas de produção protegidas normalmente.
// ---------------------------------------------------------------------
func rotas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/produto", handlers.ProdutoHandler)
	mux.HandleFunc("/movimentacao", handlers.MovimentacaoHandler)
	mux.HandleFunc("/saldo", handlers.SaldoHandler)
	return mux
}

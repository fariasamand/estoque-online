package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"estoque-ia/models"

	"gorm.io/gorm"
)

// tempoReserva é quanto tempo um pedido segura o estoque enquanto o
// cliente paga pelo WhatsApp. Depois disso as unidades voltam a ficar
// disponíveis sozinhas.
//
// Dica para testar: troque temporariamente por 1 * time.Minute.
const tempoReserva = 15 * time.Minute

// mutexEstoque garante que só um handler por vez execute a sequência
// "calcular disponível -> conferir -> gravar". Sem ele, dois clientes
// que clicam no mesmo instante veem o mesmo disponível e os dois
// reservam — foi o que o teste de concorrência mostrou.
//
// ATENÇÃO: se você já declarou "var mutexEstoque sync.Mutex" no
// handlers.go, apague a declaração de lá (e o "sync" do import de lá).
// Ela precisa existir em um lugar só dentro do pacote.
var mutexEstoque sync.Mutex

type itemRecebido struct {
	ProdutoID  uint `json:"produto_id"`
	Quantidade int  `json:"quantidade"`
}

type pedidoRecebido struct {
	Cliente     string         `json:"cliente"`
	Telefone    string         `json:"telefone"`
	TipoEntrega string         `json:"tipo_entrega"`
	Endereco    string         `json:"endereco"`
	Observacao  string         `json:"observacao"`
	Itens       []itemRecebido `json:"itens"`
}

// ---------------------------------------------------------------------
// Cálculo do disponível
//
//   saldo real  = entradas - saídas      (o que está na prateleira)
//   reservado   = itens de pedidos "novo" ainda dentro do prazo
//   disponível  = saldo real - reservado (o que pode ser vendido agora)
// ---------------------------------------------------------------------

// calcularReservadoTx soma quantas unidades de um produto estão presas
// em pedidos com reserva ativa. Reservas vencidas simplesmente deixam
// de contar — ninguém precisa apagá-las.
//
// ignorarPedidoID deixa um pedido de fora da soma. É usado na
// confirmação, para o pedido não disputar estoque com ele mesmo.
// Passe 0 para considerar todos.
func calcularReservadoTx(banco *gorm.DB, produtoID uint, ignorarPedidoID uint) int {
	var pedidos []models.Pedido
	banco.Preload("Itens").Where("status = ?", "novo").Find(&pedidos)

	agora := time.Now()
	total := 0
	for _, p := range pedidos {
		if p.ID == ignorarPedidoID || !p.ReservaAtiva(agora) {
			continue
		}
		for _, item := range p.Itens {
			if item.ProdutoID == produtoID {
				total += item.Quantidade
			}
		}
	}
	return total
}

func calcularDisponivelTx(banco *gorm.DB, produtoID uint, ignorarPedidoID uint) int {
	return calcularSaldoTx(banco, produtoID) - calcularReservadoTx(banco, produtoID, ignorarPedidoID)
}

// calcularDisponivel é o atalho para o catálogo usar.
func calcularDisponivel(produtoID uint) int {
	return calcularDisponivelTx(db, produtoID, 0)
}

// expirarReservasVencidas muda para "expirado" os pedidos "novo" cujo
// prazo já passou.
//
// Não precisa rodar em segundo plano nem de hora em hora: o status só
// aparece na tela de pedidos do admin, então basta chamar esta função
// no começo do handler que monta essa lista. Sempre que alguém olhar,
// estará atualizado.
//
// Pedidos antigos, sem prazo (ExpiraEm vazio), são deixados em paz.
func expirarReservasVencidas() {
	fmt.Println("conferindo reservas vencidas...") // diagnóstico: pode tirar depois

	var pedidos []models.Pedido
	db.Where("status = ?", "novo").Find(&pedidos)

	agora := time.Now()
	var vencidos []uint
	for _, p := range pedidos {
		if !p.ExpiraEm.IsZero() && !p.ExpiraEm.After(agora) {
			vencidos = append(vencidos, p.ID)
		}
	}

	if len(vencidos) > 0 {
		// O "status = novo" no filtro é proteção: se alguém confirmou o
		// pedido no mesmo instante, ele já não é mais "novo" e não será
		// sobrescrito para expirado.
		db.Model(&models.Pedido{}).
			Where("id IN ? AND status = ?", vencidos, "novo").
			Update("status", "expirado")
		fmt.Printf("pedidos expirados: %v\n", vencidos) // diagnóstico: pode tirar depois
	}
}

// ---------------------------------------------------------------------
// Cliente faz o pedido no site -> vira reserva por tempoReserva
// ---------------------------------------------------------------------

func PedidoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var entrada pedidoRecebido
	if err := json.NewDecoder(r.Body).Decode(&entrada); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	if entrada.Cliente == "" || entrada.Telefone == "" {
		http.Error(w, "informe nome e telefone", http.StatusBadRequest)
		return
	}
	if len(entrada.Itens) == 0 {
		http.Error(w, "pedido sem itens", http.StatusBadRequest)
		return
	}
	if entrada.TipoEntrega != "retirada" && entrada.TipoEntrega != "caravana" {
		http.Error(w, "tipo de entrega inválido", http.StatusBadRequest)
		return
	}

	// A partir daqui: consultar disponível, conferir e gravar precisam
	// acontecer sem ninguém no meio. Só um pedido passa por vez.
	mutexEstoque.Lock()
	defer mutexEstoque.Unlock()

	agora := time.Now()
	pedido := models.Pedido{
		Cliente:     entrada.Cliente,
		Telefone:    entrada.Telefone,
		TipoEntrega: entrada.TipoEntrega,
		Endereco:    entrada.Endereco,
		Observacao:  entrada.Observacao,
		Status:      "novo",
		CriadoEm:    agora,
		ExpiraEm:    agora.Add(tempoReserva),
	}

	// Se o carrinho mandar o mesmo produto em duas linhas, a segunda
	// linha precisa enxergar o que a primeira já pegou.
	jaNoPedido := map[uint]int{}

	for _, i := range entrada.Itens {
		if i.Quantidade < 1 {
			http.Error(w, "quantidade inválida", http.StatusBadRequest)
			return
		}

		var produto models.Produto
		if err := db.First(&produto, i.ProdutoID).Error; err != nil {
			http.Error(w, "produto não encontrado", http.StatusBadRequest)
			return
		}

		disponivel := calcularDisponivel(produto.ID) - jaNoPedido[produto.ID]
		if i.Quantidade > disponivel {
			if disponivel < 0 {
				disponivel = 0
			}
			http.Error(w,
				fmt.Sprintf("%s: disponível apenas %d unidade(s)", produto.Nome, disponivel),
				http.StatusUnprocessableEntity)
			return
		}
		jaNoPedido[produto.ID] += i.Quantidade

		pedido.Itens = append(pedido.Itens, models.ItemPedido{
			ProdutoID:   produto.ID,
			NomeProduto: produto.Nome,
			PrecoUnit:   produto.Preco,
			Quantidade:  i.Quantidade,
		})
		pedido.Total += produto.Preco * float64(i.Quantidade)
	}

	if err := db.Create(&pedido).Error; err != nil {
		http.Error(w, "erro ao salvar pedido", http.StatusInternalServerError)
		fmt.Println("erro ao salvar pedido:", err)
		return
	}

	fmt.Printf("NOVO PEDIDO #%d — %s — R$ %.2f — reservado até %s\n",
		pedido.ID, pedido.Cliente, pedido.Total, pedido.ExpiraEm.Format("15:04"))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":        pedido.ID,
		"total":     pedido.Total,
		"expira_em": pedido.ExpiraEm.Format("15:04"), // para a mensagem do WhatsApp
	})
}

// ---------------------------------------------------------------------
// Loja confirma o pagamento -> reserva vira baixa real
// ---------------------------------------------------------------------

func ConfirmarPedidoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 1 {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}

	mutexEstoque.Lock()
	defer mutexEstoque.Unlock()

	err = db.Transaction(func(tx *gorm.DB) error {
		var pedido models.Pedido
		if err := tx.Preload("Itens").First(&pedido, id).Error; err != nil {
			return fmt.Errorf("pedido não encontrado")
		}

		// "expirado" também pode ser confirmado: o cliente pagou atrasado.
		// Se ainda houver estoque livre, a venda passa normalmente.
		if pedido.Status != "novo" && pedido.Status != "expirado" {
			return fmt.Errorf("pedido já está como %s", pedido.Status)
		}

		expirou := pedido.Status == "expirado" || !pedido.ReservaAtiva(time.Now())

		for _, item := range pedido.Itens {
			// Disponível descontando as reservas dos OUTROS pedidos.
			// Se a reserva deste ainda vale, as unidades dele estão aqui.
			// Se expirou, ele só leva se ninguém tiver reservado antes.
			disponivel := calcularDisponivelTx(tx, item.ProdutoID, pedido.ID)
			if item.Quantidade > disponivel {
				if disponivel < 0 {
					disponivel = 0
				}
				if expirou {
					return fmt.Errorf("%s: a reserva deste pedido expirou e restam só %d unidade(s) livres; o pedido pede %d",
						item.NomeProduto, disponivel, item.Quantidade)
				}
				return fmt.Errorf("%s: disponível %d, pedido pede %d",
					item.NomeProduto, disponivel, item.Quantidade)
			}

			mov := models.Movimentacao{
				ProdutoID:  item.ProdutoID,
				Tipo:       "saida",
				Quantidade: item.Quantidade,
			}
			if err := tx.Omit("Produto").Create(&mov).Error; err != nil {
				return err
			}
		}

		return tx.Model(&models.Pedido{}).
			Where("id = ?", pedido.ID).
			Update("status", "confirmado").Error
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	fmt.Fprintf(w, "pedido %d confirmado e baixado do estoque", id)
}

// ---------------------------------------------------------------------
// Cancelar: sem mudança. Cancelar um pedido "novo" libera a reserva na
// hora (ele deixa de ser "novo"); cancelar um confirmado faz o estorno.
// ---------------------------------------------------------------------

func CancelarPedidoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 1 {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var pedido models.Pedido
		if err := tx.Preload("Itens").First(&pedido, id).Error; err != nil {
			return fmt.Errorf("pedido não encontrado")
		}

		if pedido.Status == "cancelado" {
			return fmt.Errorf("pedido já está cancelado")
		}

		if pedido.Status == "confirmado" {
			for _, item := range pedido.Itens {
				estorno := models.Movimentacao{
					ProdutoID:  item.ProdutoID,
					Tipo:       "entrada",
					Quantidade: item.Quantidade,
				}
				if err := tx.Omit("Produto").Create(&estorno).Error; err != nil {
					return err
				}
			}
		}

		return tx.Model(&models.Pedido{}).
			Where("id = ?", pedido.ID).
			Update("status", "cancelado").Error
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	fmt.Fprintf(w, "pedido %d cancelado", id)
}

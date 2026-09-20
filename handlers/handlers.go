package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"estoque-ia/models"

	"gorm.io/gorm"
)

var db *gorm.DB

func Iniciar(banco *gorm.DB) {
	db = banco
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "estou vivo")
}

func ProdutoHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listarProdutos(w, r)
	case http.MethodPost:
		criarProduto(w, r)
	default:
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
	}
}

func listarProdutos(w http.ResponseWriter, r *http.Request) {
	var produtos []models.Produto
	result := db.Find(&produtos)
	if result.Error != nil {
		http.Error(w, "erro ao consultar produtos", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(produtos)
}

func criarProduto(w http.ResponseWriter, r *http.Request) {
	var p models.Produto
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	result := db.Create(&p)
	if result.Error != nil {
		http.Error(w, "erro ao salvar produto", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func MovimentacaoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido, use POST", http.StatusMethodNotAllowed)
		return
	}

	var mov models.Movimentacao
	err := json.NewDecoder(r.Body).Decode(&mov)
	if err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	var produto models.Produto
	result := db.First(&produto, mov.ProdutoID)
	if result.Error != nil {
		http.Error(w, "produto não encontrado", http.StatusBadRequest)
		return
	}
	if mov.Tipo == "saida" {
		saldoAtual := calcularSaldo(mov.ProdutoID)
		if mov.Quantidade > saldoAtual {
			http.Error(w, fmt.Sprintf("estoque insuficiente: saldo atual é %d", saldoAtual), http.StatusUnprocessableEntity)
			return
		}
	}

	result = db.Omit("Produto").Create(&mov)
	if result.Error != nil {
		http.Error(w, "erro ao salvar no banco", http.StatusInternalServerError)
		return
	}

	result = db.Preload("Produto").First(&mov, mov.ID)
	if result.Error != nil {
		fmt.Println("erro ao recarregar movimentação com produto:", result.Error)
	}

	fmt.Printf("Movimentação salva no banco: %+v\n", mov)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mov)
}

func calcularSaldoTx(banco *gorm.DB, produtoID uint) int {
	var movimentacoes []models.Movimentacao
	banco.Where("produto_id = ?", produtoID).Find(&movimentacoes)

	saldo := 0
	for _, mov := range movimentacoes {
		if mov.Tipo == "entrada" {
			saldo += mov.Quantidade
		} else if mov.Tipo == "saida" {
			saldo -= mov.Quantidade
		}
	}
	return saldo
}

func calcularSaldo(produtoID uint) int {
	return calcularSaldoTx(db, produtoID)
}

func SaldoHandler(w http.ResponseWriter, r *http.Request) {
	var produtos []models.Produto
	result := db.Find(&produtos)
	if result.Error != nil {
		http.Error(w, "erro ao consultar produtos", http.StatusInternalServerError)
		return
	}

	var saldos []models.SaldoProduto

	for _, produto := range produtos {
		saldos = append(saldos, models.SaldoProduto{
			ProdutoID: produto.ID,
			Nome:      produto.Nome,
			Saldo:     calcularSaldo(produto.ID),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saldos)
}

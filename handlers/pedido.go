package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"estoque-ia/models"

	"gorm.io/gorm"
)

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

	pedido := models.Pedido{
		Cliente:     entrada.Cliente,
		Telefone:    entrada.Telefone,
		TipoEntrega: entrada.TipoEntrega,
		Endereco:    entrada.Endereco,
		Observacao:  entrada.Observacao,
		Status:      "novo",
		CriadoEm:    time.Now(),
	}

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

		saldo := calcularSaldo(produto.ID)
		if i.Quantidade > saldo {
			http.Error(w,
				fmt.Sprintf("%s: disponível apenas %d unidade(s)", produto.Nome, saldo),
				http.StatusUnprocessableEntity)
			return
		}

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

	fmt.Printf("NOVO PEDIDO #%d — %s — R$ %.2f\n", pedido.ID, pedido.Cliente, pedido.Total)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":    pedido.ID,
		"total": pedido.Total,
	})
}

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

	err = db.Transaction(func(tx *gorm.DB) error {
		var pedido models.Pedido
		if err := tx.Preload("Itens").First(&pedido, id).Error; err != nil {
			return fmt.Errorf("pedido não encontrado")
		}

		if pedido.Status != "novo" {
			return fmt.Errorf("pedido já está como %s", pedido.Status)
		}

		for _, item := range pedido.Itens {
			saldo := calcularSaldoTx(tx, item.ProdutoID)
			if item.Quantidade > saldo {
				return fmt.Errorf("%s: saldo atual é %d, pedido pede %d",
					item.NomeProduto, saldo, item.Quantidade)
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

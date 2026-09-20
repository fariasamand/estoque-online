package models

import "time"

type Pedido struct {
	ID          uint         `json:"id" gorm:"primaryKey"`
	Cliente     string       `json:"cliente"`
	Telefone    string       `json:"telefone"`
	TipoEntrega string       `json:"tipo_entrega"`
	Endereco    string       `json:"endereco"`
	Observacao  string       `json:"observacao"`
	Status      string       `json:"status"`
	Total       float64      `json:"total"`
	CriadoEm    time.Time    `json:"criado_em"`
	Itens       []ItemPedido `json:"itens" gorm:"foreignKey:PedidoID"`
}

type ItemPedido struct {
	ID          uint    `json:"id" gorm:"primaryKey"`
	PedidoID    uint    `json:"pedido_id"`
	ProdutoID   uint    `json:"produto_id"`
	NomeProduto string  `json:"nome_produto"`
	PrecoUnit   float64 `json:"preco_unit"`
	Quantidade  int     `json:"quantidade"`
}

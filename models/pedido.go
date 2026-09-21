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
	ExpiraEm    time.Time    `json:"expira_em"` // NOVO: até quando o pedido segura o estoque
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

// ReservaAtiva diz se o pedido ainda está segurando estoque:
// ele precisa estar aguardando pagamento ("novo") e dentro do prazo.
//
// Pedidos antigos, criados antes desta mudança, têm ExpiraEm vazio
// (ano 0001), então nunca contam como reserva — nada quebra.
func (p Pedido) ReservaAtiva(agora time.Time) bool {
	return p.Status == "novo" && p.ExpiraEm.After(agora)
}

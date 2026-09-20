package models

type Produto struct {
	ID        uint    `json:"id" gorm:"primaryKey"`
	Nome      string  `json:"nome"`
	Descricao string  `json:"descricao"`
	Categoria string  `json:"categoria"`
	Preco     float64 `json:"preco"`
	Imagem    string  `json:"imagem"`
}

type Movimentacao struct {
	ID         uint    `json:"id" gorm:"primaryKey"`
	ProdutoID  uint    `json:"produto_id"`
	Produto    Produto `json:"produto" gorm:"foreignKey:ProdutoID"`
	Tipo       string  `json:"tipo"`
	Quantidade int     `json:"quantidade"`
}

type SaldoProduto struct {
	ProdutoID uint   `json:"produto_id"`
	Nome      string `json:"nome"`
	Saldo     int    `json:"saldo"`
}

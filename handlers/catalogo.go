package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"estoque-ia/models"
)

var CategoriasLoja = []string{"Banho", "Cama", "Mesa", "Decoração"}

type ItemCatalogo struct {
	ID         uint
	Nome       string
	Descricao  string
	Categoria  string
	Preco      string
	PrecoNum   float64
	Imagem     string
	Saldo      int
	Disponivel bool
}

type SecaoCatalogo struct {
	Nome  string
	Itens []ItemCatalogo
}

type PaginaCatalogo struct {
	Loja   string
	Secoes []SecaoCatalogo
	Total  int
}

func CatalogoHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/catalogo.html")
	if err != nil {
		http.Error(w, "erro ao carregar página", http.StatusInternalServerError)
		fmt.Println("erro no template:", err)
		return
	}

	var produtos []models.Produto
	if err := db.Order("nome").Find(&produtos).Error; err != nil {
		http.Error(w, "erro ao carregar catálogo", http.StatusInternalServerError)
		fmt.Println("erro no banco:", err)
		return
	}

	porCategoria := make(map[string][]ItemCatalogo)

	for _, p := range produtos {
		saldo := calcularSaldo(p.ID)

		item := ItemCatalogo{
			ID:         p.ID,
			Nome:       p.Nome,
			Descricao:  p.Descricao,
			Categoria:  p.Categoria,
			Preco:      formatarPreco(p.Preco),
			PrecoNum:   p.Preco,
			Imagem:     p.Imagem,
			Saldo:      saldo,
			Disponivel: saldo > 0,
		}

		cat := p.Categoria
		if cat == "" {
			cat = "Outros"
		}
		porCategoria[cat] = append(porCategoria[cat], item)
	}

	var secoes []SecaoCatalogo
	for _, nome := range CategoriasLoja {
		if itens, existe := porCategoria[nome]; existe {
			secoes = append(secoes, SecaoCatalogo{Nome: nome, Itens: itens})
		}
	}
	if itens, existe := porCategoria["Outros"]; existe {
		secoes = append(secoes, SecaoCatalogo{Nome: "Outros", Itens: itens})
	}

	pagina := PaginaCatalogo{
		Loja:   "Loja Sem Nome",
		Secoes: secoes,
		Total:  len(produtos),
	}

	if err := tmpl.Execute(w, pagina); err != nil {
		fmt.Println("erro ao executar template:", err)
	}
}

func formatarPreco(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}

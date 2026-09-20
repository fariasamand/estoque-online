package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"estoque-ia/models"
)

func PainelAdminHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/admin.html")
	if err != nil {
		http.Error(w, "erro ao carregar página", http.StatusInternalServerError)
		fmt.Println("erro no template:", err)
		return
	}
	tmpl.Execute(w, map[string]string{"Loja": "Loja Sem Nome"})
}

// lista produtos com saldo, para a tela de admin
func AdminProdutosHandler(w http.ResponseWriter, r *http.Request) {
	var produtos []models.Produto
	if err := db.Order("nome").Find(&produtos).Error; err != nil {
		http.Error(w, "erro ao consultar produtos", http.StatusInternalServerError)
		return
	}

	type linha struct {
		ID        uint    `json:"id"`
		Nome      string  `json:"nome"`
		Descricao string  `json:"descricao"`
		Categoria string  `json:"categoria"`
		Preco     float64 `json:"preco"`
		Imagem    string  `json:"imagem"`
		Saldo     int     `json:"saldo"`
	}

	lista := make([]linha, 0, len(produtos))
	for _, p := range produtos {
		lista = append(lista, linha{
			ID: p.ID, Nome: p.Nome, Descricao: p.Descricao,
			Categoria: p.Categoria,
			Preco:     p.Preco, Imagem: p.Imagem, Saldo: calcularSaldo(p.ID),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(lista)
}

var naoPermitido = regexp.MustCompile(`[^a-z0-9.-]+`)

func nomeSeguro(original string) string {
	nome := strings.ToLower(filepath.Base(original))
	nome = strings.ReplaceAll(nome, " ", "-")
	nome = naoPermitido.ReplaceAllString(nome, "")
	return nome
}

// cria ou atualiza produto, com upload opcional de foto
func AdminSalvarProdutoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// 10 MB em memória, o resto vai para disco temporário
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}

	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		http.Error(w, "informe o nome do produto", http.StatusBadRequest)
		return
	}

	preco, err := strconv.ParseFloat(strings.Replace(r.FormValue("preco"), ",", ".", 1), 64)
	if err != nil || preco <= 0 {
		http.Error(w, "preço inválido", http.StatusBadRequest)
		return
	}

	var produto models.Produto

	// se veio id, é edição
	if idTexto := r.FormValue("id"); idTexto != "" && idTexto != "0" {
		id, err := strconv.Atoi(idTexto)
		if err != nil {
			http.Error(w, "id inválido", http.StatusBadRequest)
			return
		}
		if err := db.First(&produto, id).Error; err != nil {
			http.Error(w, "produto não encontrado", http.StatusBadRequest)
			return
		}
	}

	produto.Nome = nome
	produto.Descricao = strings.TrimSpace(r.FormValue("descricao"))
	produto.Categoria = strings.TrimSpace(r.FormValue("categoria"))
	produto.Preco = preco

	// foto é opcional: sem arquivo novo, mantém a atual
	arquivo, cabecalho, err := r.FormFile("foto")
	if err == nil {
		defer arquivo.Close()

		ext := strings.ToLower(filepath.Ext(cabecalho.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			http.Error(w, "formato de imagem não aceito (use jpg, png ou webp)", http.StatusBadRequest)
			return
		}
		if cabecalho.Size > 8<<20 {
			http.Error(w, "imagem muito grande (máximo 8 MB)", http.StatusBadRequest)
			return
		}

		if err := os.MkdirAll("static/fotos", 0o755); err != nil {
			http.Error(w, "erro ao preparar pasta de fotos", http.StatusInternalServerError)
			return
		}

		nomeArquivo := nomeSeguro(cabecalho.Filename)
		destino := filepath.Join("static", "fotos", nomeArquivo)

		saida, err := os.Create(destino)
		if err != nil {
			http.Error(w, "erro ao salvar imagem", http.StatusInternalServerError)
			fmt.Println("erro ao criar arquivo:", err)
			return
		}
		defer saida.Close()

		if _, err := io.Copy(saida, arquivo); err != nil {
			http.Error(w, "erro ao gravar imagem", http.StatusInternalServerError)
			return
		}

		produto.Imagem = nomeArquivo
	}

	if produto.ID == 0 {
		err = db.Create(&produto).Error
	} else {
		err = db.Save(&produto).Error
	}
	if err != nil {
		http.Error(w, "erro ao salvar produto", http.StatusInternalServerError)
		fmt.Println("erro ao salvar produto:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": produto.ID, "nome": produto.Nome})
}

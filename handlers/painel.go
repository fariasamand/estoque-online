package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"estoque-ia/models"
)

func PainelPedidosHandler(w http.ResponseWriter, r *http.Request) {
	expirarReservasVencidas()
	tmpl, err := template.ParseFiles("templates/pedidos.html")
	if err != nil {
		http.Error(w, "erro ao carregar página", http.StatusInternalServerError)
		fmt.Println("erro no template:", err)
		return
	}

	tmpl.Execute(w, map[string]string{"Loja": "Loja Sem Nome"})
}

func ListaPedidosHandler(w http.ResponseWriter, r *http.Request) {
	expirarReservasVencidas()
	var pedidos []models.Pedido

	err := db.Preload("Itens").Order("id desc").Limit(100).Find(&pedidos).Error
	if err != nil {
		http.Error(w, "erro ao consultar pedidos", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pedidos)
}

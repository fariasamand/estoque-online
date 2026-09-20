package main

import (
	"fmt"
	"net/http"

	"estoque-ia/database"
	"estoque-ia/handlers"
)

func main() {
	banco := database.Conectar()
	handlers.Iniciar(banco)

	// --- público: o que o cliente acessa ---
	http.HandleFunc("/health", handlers.HealthHandler)
	http.HandleFunc("/catalogo", handlers.CatalogoHandler)
	http.HandleFunc("/pedidos", handlers.PedidoHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// --- restrito: só a loja ---
	http.HandleFunc("/produto", handlers.Protegido(handlers.ProdutoHandler))
	http.HandleFunc("/movimentacao", handlers.Protegido(handlers.MovimentacaoHandler))
	http.HandleFunc("/saldo", handlers.Protegido(handlers.SaldoHandler))

	http.HandleFunc("/admin", handlers.Protegido(handlers.PainelAdminHandler))
	http.HandleFunc("/admin/produtos/lista", handlers.Protegido(handlers.AdminProdutosHandler))
	http.HandleFunc("/admin/produtos/salvar", handlers.Protegido(handlers.AdminSalvarProdutoHandler))

	http.HandleFunc("/admin/pedidos", handlers.Protegido(handlers.PainelPedidosHandler))

	http.HandleFunc("/admin/pedidos/lista", handlers.Protegido(handlers.ListaPedidosHandler))
	http.HandleFunc("/admin/pedidos/confirmar", handlers.Protegido(handlers.ConfirmarPedidoHandler))
	http.HandleFunc("/admin/pedidos/cancelar", handlers.Protegido(handlers.CancelarPedidoHandler))

	fmt.Println("Servidor rodando em http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

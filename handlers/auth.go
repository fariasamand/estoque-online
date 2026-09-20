package handlers

import (
	"crypto/subtle"
	"net/http"
	"os"
)

func Protegido(prox http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		usuarioOK := os.Getenv("ADMIN_USUARIO")
		senhaOK := os.Getenv("ADMIN_SENHA")

		if usuarioOK == "" || senhaOK == "" {
			http.Error(w, "acesso administrativo não configurado", http.StatusInternalServerError)
			return
		}

		usuario, senha, ok := r.BasicAuth()

		if !ok ||
			subtle.ConstantTimeCompare([]byte(usuario), []byte(usuarioOK)) != 1 ||
			subtle.ConstantTimeCompare([]byte(senha), []byte(senhaOK)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Area restrita"`)
			http.Error(w, "acesso negado", http.StatusUnauthorized)
			return
		}

		prox(w, r)
	}
}

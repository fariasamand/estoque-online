package database

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"estoque-ia/models"
)

var DB *gorm.DB

func Conectar() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("estoque.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("erro ao abrir banco:", err)
	}

	err = db.AutoMigrate(
		&models.Produto{},
		&models.Movimentacao{},
		&models.Pedido{},
		&models.ItemPedido{},
	)
	if err != nil {
		log.Fatal("erro ao migrar tabelas:", err)
	}

	DB = db
	return db
}

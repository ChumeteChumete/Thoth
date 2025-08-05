package storage

import (
    "os"
    "testing"
	"github.com/joho/godotenv"
	"log"
)

func TestSaveAndGetMessage(t *testing.T) {
    if err := godotenv.Load("../../.env"); err != nil {
        log.Fatal("Error loading .env file")
    }
    connStr := os.Getenv("THOTH_DB_CONN")
    if connStr == "" {
        t.Fatal("THOTH_DB_CONN не задан")
    }

    store, err := NewStorage(connStr)
    if err != nil {
        t.Fatalf("Ошибка подключения к базе: %v", err)
    }
    defer store.Close()

    msg := Message{
        Username: "testuser",
        Content:  "Тестовое сообщение",
    }

    if err := store.SaveMessage(msg); err != nil {
        t.Fatalf("Ошибка сохранения сообщения: %v", err)
    }

    messages, err := store.GetRecentMessages(1)
    if err != nil {
        t.Fatalf("Ошибка получения сообщений: %v", err)
    }
    if len(messages) == 0 {
        t.Fatal("Нет сообщений в базе")
    }
    got := messages[0]
    if got.Username != msg.Username || got.Content != msg.Content {
        t.Errorf("Ожидалось: %+v, Получено: %+v", msg, got)
    }
    if got.CreatedAt.IsZero() {
        t.Error("CreatedAt не установлен")
    }
}
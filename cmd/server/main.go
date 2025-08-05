package main

import (
    "context"
    "os/signal"
    "syscall"
    "time"
    "log"
    "net/http"
    "os"
    "github.com/joho/godotenv"

    "Thoth/internal/handlers"
    "Thoth/internal/websocket"
    "Thoth/internal/storage"
)

func main() {
    // Загружаем переменные окружения из .env
    if err := godotenv.Load(); err != nil {
        log.Println("Файл .env не найден, переменные окружения будут взяты из системы")
    }

    connStr := os.Getenv("THOTH_DB_CONN")
    if connStr == "" {
        log.Fatal("Не задана переменная окружения THOTH_DB_CONN")
    }
    store, err := storage.NewStorage(connStr)
    if err != nil {
        log.Fatalf("Ошибка подключения к базе: %v", err)
    }
    defer store.Close()
    // Создаем Hub - центральный диспетчер чата
    hub := websocket.NewHub()
    
    // Запускаем Hub в отдельной горутине
    go hub.Run()
    
    // Создаем обработчики HTTP запросов
    chatHandler := handlers.NewChatHandler(hub, store)
    
    // Настраиваем маршруты
    http.HandleFunc("/", serveHome)                    // Главная страница
    http.HandleFunc("/ws", chatHandler.ServeWS)        // WebSocket подключения
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))
    
    // Запускаем HTTP сервер
    srv := &http.Server{Addr: ":8080"}

    go func() {
        log.Println("Сервер запущен")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Ошибка запуска сервера: %v", err)
        }
    }()

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    <-ctx.Done()
    log.Println("Получен сигнал завершения")

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("Ошибка при завершении сервера: %v", err)
    }

    hub.Stop()
    log.Println("Сервер остановлен")
}

// serveHome отдает главную страницу
func serveHome(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.Error(w, "Страница не найдена", http.StatusNotFound)
        return
    }
    
    if r.Method != "GET" {
        http.Error(w, "Метод не разрешен", http.StatusMethodNotAllowed)
        return
    }
    
    // Отдаем index.html
    http.ServeFile(w, r, "web/static/index.html")
}
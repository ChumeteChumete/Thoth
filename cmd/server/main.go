package main

import (
    "context"
    "crypto/tls"
    "fmt"
    "net"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    "log"
    "github.com/joho/godotenv"

    "Thoth/internal/handlers"
    "Thoth/internal/websocket"
    "Thoth/internal/storage"
)

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    
    // Загружаем переменные окружения из .env
    if err := godotenv.Load(); err != nil {
        log.Println("⚠️  Файл .env не найден, переменные окружения будут взяты из системы")
    }

    // Диагностическая информация
    log.Println("🚀 Запуск Thoth Chat Server")
    log.Printf("📁 Рабочая директория: %s", getCurrentDir())
    
    // Проверяем сертификаты
    if !checkCertificates() {
        log.Println("⚠️  SSL сертификаты не найдены или невалидны")
    }

    // Проверяем базу данных
    connStr := os.Getenv("THOTH_DB_CONN")
    if connStr == "" {
        log.Fatal("❌ Не задана переменная окружения THOTH_DB_CONN")
    }
    
    store, err := storage.NewStorage(connStr)
    if err != nil {
        log.Fatalf("❌ Ошибка подключения к базе: %v", err)
    }
    defer store.Close()
    log.Println("✅ Подключение к базе данных установлено")

    // Создаем Hub - центральный диспетчер чата
    hub := websocket.NewHub()
    
    // Запускаем Hub в отдельной горутине
    go hub.Run()
    log.Println("✅ WebSocket Hub запущен")
    
    // Создаем обработчики HTTP запросов
    chatHandler := handlers.NewChatHandler(hub, store)
    
    // Настраиваем маршруты
    http.HandleFunc("/", serveHome)
    http.HandleFunc("/ws", chatHandler.ServeWS)
    http.HandleFunc("/health", healthCheck)
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))
    
    // Настраиваем сервер
    srv := &http.Server{
        Addr:         "0.0.0.0:8443",
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Настраиваем TLS
    tlsConfig, err := setupTLS()
    if err != nil {
        log.Fatalf("❌ Ошибка настройки TLS: %v", err)
    }
    srv.TLSConfig = tlsConfig

    // Диагностика сети
    diagnoseNetwork()

    // Запускаем HTTP сервер
    go func() {
        log.Println("🌐 Сервер запущен на https://localhost:8443")
        log.Println("🌐 Внешний доступ: https://thoth-webrtc.duckdns.org:8443")
        log.Println("📊 Health check: https://localhost:8443/health")
        
        if err := srv.ListenAndServeTLS("certs/server.crt", "certs/server.key"); err != nil && err != http.ErrServerClosed {
            log.Fatalf("❌ Ошибка запуска HTTPS сервера: %v", err)
        }
    }()

    // Дополнительный HTTP сервер для редиректа (опционально)
    go func() {
        httpSrv := &http.Server{
            Addr: ":8080",
            Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                target := "https://" + r.Host + ":8443" + r.URL.Path
                if len(r.URL.RawQuery) > 0 {
                    target += "?" + r.URL.RawQuery
                }
                log.Printf("🔄 Редирект HTTP -> HTTPS: %s", target)
                http.Redirect(w, r, target, http.StatusPermanentRedirect)
            }),
        }
        
        log.Println("🔄 HTTP редирект сервер запущен на :8080")
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Printf("⚠️  HTTP редирект сервер: %v", err)
        }
    }()

    // Graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    <-ctx.Done()
    log.Println("🛑 Получен сигнал завершения")

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("❌ Ошибка при завершении сервера: %v", err)
    }

    hub.Stop()
    log.Println("✅ Сервер остановлен")
}

func getCurrentDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return "unknown"
    }
    return dir
}

func checkCertificates() bool {
    certFile := "certs/server.crt"
    keyFile := "certs/server.key"
    
    // Проверяем существование файлов
    if _, err := os.Stat(certFile); os.IsNotExist(err) {
        log.Printf("❌ Сертификат не найден: %s", certFile)
        return false
    }
    
    if _, err := os.Stat(keyFile); os.IsNotExist(err) {
        log.Printf("❌ Ключ не найден: %s", keyFile)
        return false
    }
    
    // Проверяем валидность сертификата
    _, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        log.Printf("❌ Ошибка загрузки сертификата: %v", err)
        return false
    }
    
    log.Println("✅ SSL сертификаты найдены и валидны")
    return true
}

func setupTLS() (*tls.Config, error) {
    cert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
    if err != nil {
        return nil, fmt.Errorf("ошибка загрузки сертификата: %v", err)
    }
    
    config := &tls.Config{
        Certificates: []tls.Certificate{cert},
        ServerName:   "thoth-webrtc.duckdns.org",
        MinVersion:   tls.VersionTLS12,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
    }
    
    log.Println("✅ TLS конфигурация настроена")
    return config, nil
}

func diagnoseNetwork() {
    // Получаем локальные IP адреса
    addrs, err := net.InterfaceAddrs()
    if err != nil {
        log.Printf("⚠️  Ошибка получения сетевых интерфейсов: %v", err)
        return
    }
    
    log.Println("🌐 Сетевые интерфейсы:")
    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                log.Printf("   📍 %s", ipnet.IP)
            }
        }
    }
    
    // Проверяем доступность порта
    listener, err := net.Listen("tcp", ":8443")
    if err != nil {
        log.Printf("❌ Порт 8443 недоступен: %v", err)
    } else {
        listener.Close()
        log.Println("✅ Порт 8443 доступен")
    }
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
    status := map[string]string{
        "status":    "ok",
        "timestamp": time.Now().Format(time.RFC3339),
        "version":   "1.0.0",
        "service":   "thoth-chat",
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    
    response := fmt.Sprintf(`{
        "status": "%s",
        "timestamp": "%s",
        "version": "%s",
        "service": "%s"
    }`, status["status"], status["timestamp"], status["version"], status["service"])
    
    w.Write([]byte(response))
    
    log.Printf("🩺 Health check from %s", r.RemoteAddr)
}

// serveHome отдает главную страницу
func serveHome(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        log.Printf("❌ 404: %s", r.URL.Path)
        http.Error(w, "Страница не найдена", http.StatusNotFound)
        return
    }
    
    if r.Method != "GET" {
        log.Printf("❌ Method not allowed: %s", r.Method)
        http.Error(w, "Метод не разрешен", http.StatusMethodNotAllowed)
        return
    }
    
    log.Printf("📄 Главная страница запрошена от %s", r.RemoteAddr)
    
    // Проверяем существование файла
    filePath := "web/static/index.html"
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        log.Printf("❌ Файл не найден: %s", filePath)
        http.Error(w, "Файл index.html не найден", http.StatusNotFound)
        return
    }
    
    // Устанавливаем заголовки безопасности
    w.Header().Set("X-Frame-Options", "DENY")
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("X-XSS-Protection", "1; mode=block")
    
    // Отдаем index.html
    http.ServeFile(w, r, filePath)
}
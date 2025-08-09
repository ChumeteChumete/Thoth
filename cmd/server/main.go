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
	"log/slog"
    "github.com/joho/godotenv"

    "Thoth/internal/handlers"
    "Thoth/internal/websocket"
    "Thoth/internal/storage"
)

var mainLogger = slog.With("component", "main")

func main() {
    
    if err := godotenv.Load(); err != nil {
        mainLogger.Error("File .env not found, environment variables will be taken from the system")
		os.Exit(1)
    }

	// Для диагностики
    mainLogger.Info("Running Thoth Chat Server")
    
    // Проверяем сертификаты
    if !checkCertificates() {
        mainLogger.Error("SSL certificates not found or invalid")
    }

    // Проверяем базу данных
    connStr := os.Getenv("THOTH_DB_CONN")
    if connStr == "" {
        mainLogger.Error("Environment variable THOTH_DB_CONN is not set")
		os.Exit(1)
    }
    
    store, err := storage.NewStorage(connStr)
    if err != nil {
        mainLogger.Error("Error connecting to the database", "error", err)
		os.Exit(1)
    }
    defer store.Close()
    mainLogger.Info("Connection to the database has been established")

    // Создаем хаб
    hub := websocket.NewHub()
    
    // Запускаем хаб в отдельной горутине
    go hub.Run()
    mainLogger.Info("WebSocket Hub launched")
    
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
        mainLogger.Error("TLS setup error", "error", err)
		os.Exit(1)
    }
    srv.TLSConfig = tlsConfig

    // Диагностика сети
    diagnoseNetwork()

    // Запускаем HTTP сервер
    go func() {
        mainLogger.Info("🌐 The server is running on https://localhost:8443")
        mainLogger.Info("🌐 External access: https://thoth-webrtc.duckdns.org:8443")
        mainLogger.Info("📊 Health check: https://localhost:8443/health")
        
        if err := srv.ListenAndServeTLS("certs/server.crt", "certs/server.key"); err != nil && err != http.ErrServerClosed {
            mainLogger.Error("HTTPS server startup error", "error", err)
			os.Exit(1)
        }
    }()

    // Дополнительный HTTP сервер для редиректа
    go func() {
        httpSrv := &http.Server{
            Addr: ":8080",
            Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                target := "https://" + r.Host + ":8443" + r.URL.Path
                if len(r.URL.RawQuery) > 0 {
                    target += "?" + r.URL.RawQuery
                }
                mainLogger.Info("Redirect HTTP -> HTTPS", "target", target)
                http.Redirect(w, r, target, http.StatusPermanentRedirect)
            }),
        }
        
        mainLogger.Info("HTTP redirect server running on :8080")
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            mainLogger.Error("ERROR HTTP redirect server", "error", err)
        }
    }()

    // Graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    <-ctx.Done()
    mainLogger.Info("Termination signal received")

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        mainLogger.Error("Error terminating server", "error", err)
		os.Exit(1)
    }

    hub.Stop()
    mainLogger.Info("The server has stopped")
}

func checkCertificates() bool {
    certFile := "certs/server.crt"
    keyFile := "certs/server.key"
    
    // Проверяем существование файлов
    if _, err := os.Stat(certFile); os.IsNotExist(err) {
        mainLogger.Error("Certificate not found", "certificate", certFile)
        return false
    }
    
    if _, err := os.Stat(keyFile); os.IsNotExist(err) {
        mainLogger.Error("Key not found", "key", keyFile)
        return false
    }
    
    // Проверяем валидность сертификата
    _, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        mainLogger.Error("Error loading certificate", "error", err)
        return false
    }
    
    mainLogger.Info("SSL certificates found and valid")
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
    
    mainLogger.Info("TLS configuration is configured")
    return config, nil
}

func diagnoseNetwork() {
    // Получаем локальные IP адреса
    addrs, err := net.InterfaceAddrs()
    if err != nil {
        mainLogger.Error("Error getting network interfaces", "error", err)
        return
    }
    
    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
                mainLogger.Info("Network interfaces", "ip", ipnet.IP)
            }
        }
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
    
    mainLogger.Info("Health check", "remote", r.RemoteAddr)
}

// serveHome отдает главную страницу
func serveHome(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        mainLogger.Error("404", "path", r.URL.Path)
        http.Error(w, "Page not found", http.StatusNotFound)
        return
    }
    
    if r.Method != "GET" {
        mainLogger.Error("Method not allowed", "method", r.Method)
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    mainLogger.Info("Requested home page", "remote", r.RemoteAddr)
    
    // Проверяем существование файла
    filePath := "web/static/index.html"
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        mainLogger.Error("File not found", "file", filePath)
        http.Error(w, "File index.html not found", http.StatusNotFound)
        return
    }
    
    // Устанавливаем заголовки безопасности
    w.Header().Set("X-Frame-Options", "DENY")
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("X-XSS-Protection", "1; mode=block")
    
    // Отдаем index.html
    http.ServeFile(w, r, filePath)
}

func init() {
    handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level:     slog.LevelInfo,
        AddSource: true,
    })
    slog.SetDefault(slog.New(handler))
}
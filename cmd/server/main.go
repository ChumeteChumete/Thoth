package main

import (
    "log"
    "net/http"

    "Thoth/internal/handlers"
    "Thoth/internal/websocket"
)

func main() {
    // Создаем Hub - центральный диспетчер чата
    hub := websocket.NewHub()
    
    // Запускаем Hub в отдельной горутине
    go hub.Run()
    
    // Создаем обработчики HTTP запросов
    chatHandler := handlers.NewChatHandler(hub)
    
    // Настраиваем маршруты
    http.HandleFunc("/", serveHome)                    // Главная страница
    http.HandleFunc("/ws", chatHandler.ServeWS)        // WebSocket подключения
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))
    
    // Запускаем HTTP сервер
    log.Println("Сервер запущен на http://191.168.0.101:8080")
    
    err := http.ListenAndServe("0.0.0.0:8080", nil)
    if err != nil {
        log.Fatal("Ошибка запуска сервера: ", err)
    }
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
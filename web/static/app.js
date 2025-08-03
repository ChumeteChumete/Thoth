class ChatClient {
    constructor() {
        this.ws = null;
        this.username = '';
        this.room = '';
        this.isConnected = false;
        
        this.initElements();
        this.bindEvents();
    }
    
    initElements() {
        this.messagesContainer = document.getElementById('messages');
        this.messageInput = document.getElementById('messageInput');
        this.sendBtn = document.getElementById('sendBtn');
        this.connectBtn = document.getElementById('connectBtn');
        this.usernameInput = document.getElementById('usernameInput');
        this.roomInput = document.getElementById('roomInput');
        this.connectionForm = document.getElementById('connectionForm');
        this.usernameDisplay = document.getElementById('username-display');
        this.roomDisplay = document.getElementById('room-display');
        this.statusElement = document.getElementById('status');
    }
    
    bindEvents() {
        this.connectBtn.addEventListener('click', () => this.connect());
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.messageInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.sendMessage();
        });
    }
    
    connect() {
        if (this.isConnected || (this.ws && this.ws.readyState === WebSocket.CONNECTING)) {
            console.log('Уже подключены или подключаемся');
            return;
        }
            // Закрываем старое соединение, если есть
        if (this.ws) {
            this.ws.close();
        }
    
        this.username = this.usernameInput.value.trim() || 'Аноним';
        this.room = this.roomInput.value.trim() || 'general';
        
        const wsUrl = `ws://192.168.0.101:8080/ws?username=${encodeURIComponent(this.username)}&room=${encodeURIComponent(this.room)}`;
        
        try {
            this.ws = new WebSocket(wsUrl);
            
            this.ws.onopen = () => {
                this.onConnected();
            };
            
            this.ws.onmessage = (event) => {
                 console.log('Получено RAW сообщение:', event.data); // ДОБАВЬ ЭТО
                 const data = JSON.parse(event.data);
                 console.log('Распарсенное сообщение:', data); // И ЭТО
                 this.handleMessage(data);
            };
            
            this.ws.onclose = () => {
                this.onDisconnected();
            };
            
            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.onDisconnected();
            };
            
        } catch (error) {
            console.error('Ошибка подключения:', error);
            alert('Ошибка подключения к серверу');
        }
    }
    
    onConnected() {
        this.isConnected = true;
        this.connectionForm.classList.add('hidden');
        this.messageInput.disabled = false;
        this.sendBtn.disabled = false;
        this.usernameDisplay.textContent = this.username;
        this.roomDisplay.textContent = this.room;
        this.statusElement.textContent = 'Подключен';
        this.statusElement.className = 'status connected';
        
        this.addSystemMessage(`Подключен к комнате "${this.room}"`);
    }
    
    onDisconnected() {
        this.isConnected = false;
        this.ws = null;
        this.messageInput.disabled = true;
        this.sendBtn.disabled = true;
        this.statusElement.textContent = 'Отключен';
        this.statusElement.className = 'status disconnected';
        
        this.addSystemMessage('Соединение потеряно');
    }
    
    sendMessage() {
        if (!this.isConnected || !this.messageInput.value.trim()) return;
        
        const message = {
            type: 'chat',
            content: this.messageInput.value.trim(),
            timestamp: new Date().toISOString()
        };
        
        console.log('Отправляем сообщение:', message);
        this.ws.send(JSON.stringify(message));
        this.messageInput.value = '';
    }
    
    displayMessage(message) {
        const messageEl = document.createElement('div');
        messageEl.className = 'message';
        
        const time = new Date(message.timestamp).toLocaleTimeString('ru-RU');
        
        messageEl.innerHTML = `
            <div class="message-header">
                ${message.username}
                <span class="message-time">${time}</span>
            </div>
            <div class="message-content">${this.escapeHtml(message.content)}</div>
        `;
        
        this.messagesContainer.appendChild(messageEl);
        this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
    }
    
    addSystemMessage(text) {
        const messageEl = document.createElement('div');
        messageEl.className = 'message system-message';
        messageEl.style.fontStyle = 'italic';
        messageEl.style.color = '#666';
        messageEl.textContent = text;
        
        this.messagesContainer.appendChild(messageEl);
        this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
    }
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Запускаем чат при загрузке страницы
document.addEventListener('DOMContentLoaded', () => {
    new ChatClient();
});
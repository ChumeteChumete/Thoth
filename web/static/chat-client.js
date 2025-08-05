class ThothChatClient {
    constructor() {
        this.ws = null;
        this.username = '';
        this.room = '';
        this.isConnected = false;
        this.localStream = null;
        this.peerConnections = new Map(); // username -> RTCPeerConnection
        this.onlineUsers = new Set();
        this.broadcastingUsers = new Set(); // пользователи с включенным видео
        
        // WebRTC конфигурация
        this.rtcConfig = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };
        
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
        this.connectionOverlay = document.getElementById('connectionOverlay');
        this.usernameDisplay = document.getElementById('username-display');
        this.roomDisplay = document.getElementById('room-display');
        this.usersContainer = document.getElementById('users-container');
        this.usersCount = document.getElementById('users-count');
        this.videoToggle = document.getElementById('videoToggle');
        this.audioToggle = document.getElementById('audioToggle');
        this.videoArea = document.getElementById('videoArea');
        this.localVideo = document.getElementById('localVideo');
    }
    
    bindEvents() {
        this.connectBtn.addEventListener('click', () => this.connect());
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.messageInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.sendMessage();
        });
        this.videoToggle.addEventListener('click', () => this.toggleVideo());
        this.audioToggle.addEventListener('click', () => this.toggleAudio());
    }
    
    connect() {
        this.connectBtn.classList.add('connecting');
        this.connectBtn.textContent = 'Подключаемся...';
        this.connectBtn.disabled = true;
        
        if (this.isConnected || (this.ws && this.ws.readyState === WebSocket.CONNECTING)) {
            this.resetConnectButton();
            return;
        }
        
        if (this.ws) {
            this.ws.close();
        }
        
        this.username = this.usernameInput.value.trim() || 'Аноним';
        this.room = this.roomInput.value.trim() || 'general';
        
        const wsUrl = `wss://192.168.0.101:8443/ws?username=${encodeURIComponent(this.username)}&room=${encodeURIComponent(this.room)}`;
        
        try {
            this.ws = new WebSocket(wsUrl);
            
            this.ws.onopen = () => {
                console.log('✅ WebSocket соединение установлено');
                this.onConnected();
            };
            
            this.ws.onmessage = (event) => {
                const data = JSON.parse(event.data);
                this.handleMessage(data);
            };
            
            this.ws.onclose = (event) => {
                console.log('❌ WebSocket соединение закрыто:', event.code, event.reason);
                this.onDisconnected();
            };
            
            this.ws.onerror = (error) => {
                console.error('❌ WebSocket ошибка:', error);
                this.onDisconnected();
            };
            
        } catch (error) {
            console.error('Ошибка подключения:', error);
            alert('Ошибка подключения к серверу');
            this.resetConnectButton();
        }
    }
    
    resetConnectButton() {
        this.connectBtn.classList.remove('connecting');
        this.connectBtn.textContent = 'Подключиться';
        this.connectBtn.disabled = false;
    }
    
    onConnected() {
        this.isConnected = true;
        this.connectionOverlay.classList.add('hidden');
        this.messageInput.disabled = false;
        this.sendBtn.disabled = false;
        this.usernameDisplay.textContent = this.username;
        this.roomDisplay.textContent = this.room;
        
        this.addSystemMessage(`Подключились к комнате "${this.room}"`);
        this.addUser(this.username);
        this.resetConnectButton();
    }
    
    onDisconnected() {
        this.isConnected = false;
        this.ws = null;
        this.messageInput.disabled = true;
        this.sendBtn.disabled = true;
        
        this.addSystemMessage('Соединение потеряно');
        this.onlineUsers.clear();
        this.broadcastingUsers.clear();
        this.updateUsersList();
        this.resetConnectButton();
        
        // Закрываем все WebRTC соединения
        this.peerConnections.forEach((pc, username) => {
            console.log('🔌 Закрываем WebRTC соединение с', username);
            pc.close();
        });
        this.peerConnections.clear();
    }
    
    handleMessage(data) {
        console.log('📨 Получено сообщение:', data);
        
        if (data.type === 'chat') {
            this.displayMessage(data);
        } else if (data.type === 'user_joined') {
            this.addUser(data.username);
            this.addSystemMessage(`${data.username} присоединился к чату`);
            
            // Если у нас есть видео, сразу инициируем звонок новому пользователю
            if (this.localStream && data.username !== this.username) {
                setTimeout(() => {
                    console.log('🎥 Автоматически звоним новому пользователю:', data.username);
                    this.startVideoCall(data.username);
                }, 2000); // Увеличили задержку для стабильности
            }
            
        } else if (data.type === 'user_left') {
            this.removeUser(data.username);
            this.addSystemMessage(`${data.username} покинул чат`);
            this.closePeerConnection(data.username);
        } else if (data.type === 'users_list') {
            try {
                const users = JSON.parse(data.content);
                this.onlineUsers.clear();
                users.forEach(username => this.onlineUsers.add(username));
                this.updateUsersList();
                
                // Если у нас есть видео, звоним всем пользователям
                if (this.localStream) {
                    users.forEach(username => {
                        if (username !== this.username && !this.peerConnections.has(username)) {
                            setTimeout(() => {
                                console.log('🎥 Автоматически звоним пользователю из списка:', username);
                                this.startVideoCall(username);
                            }, Math.random() * 2000 + 500);
                        }
                    });
                }
                
            } catch (error) {
                console.error('Ошибка парсинга списка пользователей:', error);
            }
        } else if (data.type === 'webrtc_offer') {
            console.log('📞 Получен WebRTC offer от', data.username);
            this.handleWebRTCOffer(data);
        } else if (data.type === 'webrtc_answer') {
            console.log('📞 Получен WebRTC answer от', data.username);
            this.handleWebRTCAnswer(data);
        } else if (data.type === 'webrtc_candidate') {
            console.log('🧊 Получен ICE candidate от', data.username);
            this.handleWebRTCCandidate(data);
        }
    }
    
    sendMessage() {
        if (!this.isConnected || !this.messageInput.value.trim()) return;
        
        const message = {
            type: 'chat',
            content: this.messageInput.value.trim(),
            timestamp: new Date().toISOString()
        };
        
        console.log('📤 Отправляем сообщение:', message);
        this.ws.send(JSON.stringify(message));
        this.messageInput.value = '';
    }
    
    // WebRTC методы
    
    createPeerConnection(username) {
        console.log('🔗 Создаем WebRTC соединение с', username);
        const pc = new RTCPeerConnection(this.rtcConfig);
        
        // Добавляем локальный поток к соединению
        if (this.localStream) {
            this.localStream.getTracks().forEach(track => {
                console.log('➕ Добавляем трек:', track.kind, 'для пользователя:', username);
                pc.addTrack(track, this.localStream);
            });
        }
        
        // Обработчик для получения удаленного потока
        pc.ontrack = (event) => {
            console.log('🎬 Получен удаленный поток от', username, 'треков:', event.streams[0].getTracks().length);
            this.displayRemoteVideo(username, event.streams[0]);
            this.broadcastingUsers.add(username);
            this.updateUsersList();
        };
        
        // Обработчик ICE candidates
        pc.onicecandidate = (event) => {
            if (event.candidate) {
                console.log('🧊 Отправляем ICE candidate для', username);
                this.sendWebRTCMessage('webrtc_candidate', username, {
                    candidate: event.candidate
                });
            }
        };
        
        // Обработчик изменения состояния соединения
        pc.onconnectionstatechange = () => {
            console.log(`🔄 WebRTC состояние с ${username}:`, pc.connectionState);
            if (pc.connectionState === 'connected') {
                console.log(`✅ Видео-соединение с ${username} установлено`);
                this.addSystemMessage(`Видео-соединение с ${username} установлено`);
                this.broadcastingUsers.add(username);
                this.updateUsersList();
            } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed') {
                console.log(`❌ Видео-соединение с ${username} потеряно`);
                this.addSystemMessage(`Видео-соединение с ${username} потеряно`);
                this.broadcastingUsers.delete(username);
                this.updateUsersList();
                this.closePeerConnection(username);
            }
        };
        
        // Сохраняем соединение
        this.peerConnections.set(username, pc);
        return pc;
    }
    
    sendWebRTCMessage(type, targetUser, data) {
        if (!this.isConnected) {
            console.warn('⚠️ Попытка отправить WebRTC сообщение без соединения');
            return;
        }
        
        const message = {
            type: type,
            target_user: targetUser,
            webrtc_data: data,
            timestamp: new Date().toISOString()
        };
        
        console.log('📤 Отправляем WebRTC сообщение:', type, 'для', targetUser);
        this.ws.send(JSON.stringify(message));
    }
    
    async handleWebRTCOffer(data) {
        console.log('📞 Обрабатываем WebRTC offer от', data.username);
        
        // Создаем peer connection (даже если у нас нет своего видео)
        const pc = this.createPeerConnection(data.username);
        
        try {
            console.log('🔄 Устанавливаем remote description...');
            await pc.setRemoteDescription(data.webrtc_data.offer);
            
            console.log('🔄 Создаем answer...');
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            
            this.sendWebRTCMessage('webrtc_answer', data.username, {
                answer: answer
            });
            
            console.log('✅ WebRTC answer отправлен для', data.username);
            
        } catch (error) {
            console.error('❌ Ошибка при обработке offer:', error);
            this.addSystemMessage(`Ошибка видео-звонка с ${data.username}: ${error.message}`);
        }
    }
    
    async handleWebRTCAnswer(data) {
        console.log('📞 Обрабатываем WebRTC answer от', data.username);
        
        const pc = this.peerConnections.get(data.username);
        if (pc) {
            try {
                console.log('🔄 Устанавливаем remote description для answer...');
                await pc.setRemoteDescription(data.webrtc_data.answer);
                console.log('✅ WebRTC answer обработан успешно для', data.username);
            } catch (error) {
                console.error('❌ Ошибка при обработке answer:', error);
                this.addSystemMessage(`Ошибка при обработке ответа от ${data.username}: ${error.message}`);
            }
        } else {
            console.warn('⚠️ Не найдено peer connection для', data.username);
        }
    }
    
    async handleWebRTCCandidate(data) {
        console.log('🧊 Обрабатываем ICE candidate от', data.username);
        
        const pc = this.peerConnections.get(data.username);
        if (pc && pc.remoteDescription) {
            try {
                await pc.addIceCandidate(data.webrtc_data.candidate);
                console.log('✅ ICE candidate добавлен для', data.username);
            } catch (error) {
                console.error('❌ Ошибка при добавлении ICE candidate:', error);
            }
        } else {
            if (!pc) {
                console.warn('⚠️ Не найдено peer connection для', data.username);
            } else {
                console.warn('⚠️ Remote description не установлен для', data.username);
            }
        }
    }
    
    async startVideoCall(targetUsername) {
        if (targetUsername === this.username) {
            console.log('🚫 Попытка позвонить самому себе');
            return;
        }
        
        // Проверяем, есть ли уже соединение
        if (this.peerConnections.has(targetUsername)) {
            console.log('ℹ️ Соединение с', targetUsername, 'уже существует');
            return;
        }
        
        console.log('📞 Начинаем видео-звонок с', targetUsername);
        const pc = this.createPeerConnection(targetUsername);
        
        try {
            console.log('🔄 Создаем offer...');
            const offer = await pc.createOffer();
            await pc.setLocalDescription(offer);
            
            this.sendWebRTCMessage('webrtc_offer', targetUsername, {
                offer: offer
            });
            
            console.log('✅ WebRTC offer отправлен для', targetUsername);
            
        } catch (error) {
            console.error('❌ Ошибка при создании offer:', error);
            this.addSystemMessage(`Ошибка при звонке ${targetUsername}: ${error.message}`);
        }
    }
    
    closePeerConnection(username) {
        const pc = this.peerConnections.get(username);
        if (pc) {
            console.log('🔌 Закрываем peer connection с', username);
            pc.close();
            this.peerConnections.delete(username);
            
            // Удаляем видео элемент
            const videoElement = document.getElementById(`video-${username}`);
            if (videoElement) {
                videoElement.remove();
                console.log('🗑️ Удален видео элемент для', username);
            }
            
            // Обновляем статус пользователя
            this.broadcastingUsers.delete(username);
            this.updateUsersList();
        }
    }
    
    displayRemoteVideo(username, stream) {
        console.log('🎬 Отображаем удаленное видео для', username);
        
        // Проверяем, есть ли уже видео для этого пользователя
        let videoContainer = document.getElementById(`video-${username}`);
        
        if (!videoContainer) {
            // Создаем новый контейнер для видео
            videoContainer = document.createElement('div');
            videoContainer.id = `video-${username}`;
            videoContainer.className = 'video-container';
            
            const video = document.createElement('video');
            video.className = 'video-stream';
            video.autoplay = true;
            video.playsinline = true;
            
            const label = document.createElement('div');
            label.className = 'video-label';
            label.textContent = username;
            
            videoContainer.appendChild(video);
            videoContainer.appendChild(label);
            
            // Добавляем элементы управления
            this.createVideoControls(videoContainer, username);
            
            this.videoArea.appendChild(videoContainer);
            
            // Настраиваем элементы управления при первом добавлении видео
            this.setupVideoControls();
            
            console.log('✅ Создан новый видео контейнер для', username);
        }
        
        const video = videoContainer.querySelector('video');
        video.srcObject = stream;
        
        // Показываем видео область, если скрыта
        this.videoArea.classList.add('active');
        console.log('✅ Видео поток установлен для', username);
    }
    
    setupVideoControls() {
        // Кнопка полноэкранного режима
        if (!document.querySelector('.fullscreen-toggle')) {
            const fullscreenBtn = document.createElement('button');
            fullscreenBtn.className = 'fullscreen-toggle';
            fullscreenBtn.textContent = '⛶ Полный экран';
            fullscreenBtn.addEventListener('click', () => this.toggleFullscreen());
            this.videoArea.appendChild(fullscreenBtn);
        }
        
        // Обработчик ESC для выхода из полноэкранного режима
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.videoArea.classList.contains('fullscreen')) {
                this.toggleFullscreen();
            }
        });
    }
    
    toggleFullscreen() {
        const isFullscreen = this.videoArea.classList.contains('fullscreen');
        
        if (isFullscreen) {
            this.videoArea.classList.remove('fullscreen');
            const btn = this.videoArea.querySelector('.fullscreen-toggle');
            if (btn) btn.textContent = '⛶ Полный экран';
            console.log('📺 Выход из полноэкранного режима');
        } else {
            this.videoArea.classList.add('fullscreen');
            const btn = this.videoArea.querySelector('.fullscreen-toggle');
            if (btn) btn.textContent = '✕ Выйти';
            console.log('📺 Полноэкранный режим включен');
        }
    }
    
    createVideoControls(videoContainer, username) {
        const controlsOverlay = document.createElement('div');
        controlsOverlay.className = 'video-controls-overlay';
        
        // Кнопка изменения размера
        const resizeBtn = document.createElement('button');
        resizeBtn.className = 'video-control-btn';
        resizeBtn.innerHTML = '⛶';
        resizeBtn.title = 'Увеличить/уменьшить';
        resizeBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleVideoSize(videoContainer);
        });
        
        // Кнопка изменения режима отображения
        const fitBtn = document.createElement('button');
        fitBtn.className = 'video-control-btn';
        fitBtn.innerHTML = '⚏';
        fitBtn.title = 'Режим отображения';
        fitBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleVideoFit(videoContainer);
        });
        
        controlsOverlay.appendChild(resizeBtn);
        controlsOverlay.appendChild(fitBtn);
        videoContainer.appendChild(controlsOverlay);
        
        // Двойной клик для максимизации
        videoContainer.addEventListener('dblclick', () => {
            this.toggleVideoSize(videoContainer);
        });
    }
    
    toggleVideoSize(videoContainer) {
        const isMaximized = videoContainer.classList.contains('maximized');
        
        // Сначала убираем максимизацию у всех видео
        this.videoArea.querySelectorAll('.video-container').forEach(container => {
            container.classList.remove('maximized');
        });
        
        // Затем максимизируем текущее, если оно не было максимизировано
        if (!isMaximized) {
            videoContainer.classList.add('maximized');
            console.log('📺 Видео максимизировано');
        } else {
            console.log('📺 Видео возвращено к обычному размеру');
        }
    }
    
    toggleVideoFit(videoContainer) {
        const video = videoContainer.querySelector('video');
        const isContain = video.classList.contains('contain');
        
        if (isContain) {
            video.classList.remove('contain');
            console.log('📺 Режим: заполнить экран');
        } else {
            video.classList.add('contain');
            console.log('📺 Режим: показать полностью');
        }
    }
    
    displayMessage(message) {
        const messageEl = document.createElement('div');
        messageEl.className = `message ${message.username === this.username ? 'own' : ''}`;
        
        const time = new Date(message.timestamp).toLocaleTimeString('ru-RU', {
            hour: '2-digit',
            minute: '2-digit'
        });
        
        messageEl.innerHTML = `
            <div class="message-bubble">
                <div class="message-header">
                    <span>${message.username}</span>
                    <span>${time}</span>
                </div>
                <div class="message-content">${this.escapeHtml(message.content)}</div>
            </div>
        `;
        
        this.messagesContainer.appendChild(messageEl);
        this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
    }
    
    addSystemMessage(text) {
        const messageEl = document.createElement('div');
        messageEl.className = 'system-message';
        messageEl.textContent = text;
        
        this.messagesContainer.appendChild(messageEl);
        this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
    }
    
    addUser(username) {
        this.onlineUsers.add(username);
        this.updateUsersList();
    }
    
    removeUser(username) {
        this.onlineUsers.delete(username);
        this.broadcastingUsers.delete(username);
        this.updateUsersList();
    }
    
    updateUsersList() {
        this.usersCount.textContent = this.onlineUsers.size;
        this.usersContainer.innerHTML = '';
        
        Array.from(this.onlineUsers).sort().forEach(username => {
            const userEl = document.createElement('div');
            userEl.className = 'user-item';
            
            // Добавляем возможность кликнуть на других пользователей для видео-звонка
            if (username !== this.username && this.localStream) {
                userEl.classList.add('clickable');
                userEl.title = 'Нажмите для видео-звонка';
                userEl.addEventListener('click', () => {
                    console.log('👆 Клик по пользователю', username);
                    this.startVideoCall(username);
                });
            }
            
            const initial = username.charAt(0).toUpperCase();
            const isBroadcasting = this.broadcastingUsers.has(username);
            const statusClass = isBroadcasting ? 'broadcasting' : '';
            
            userEl.innerHTML = `
                <div class="user-avatar">${initial}</div>
                <span>${username}</span>
                <div class="user-status ${statusClass}"></div>
            `;
            
            this.usersContainer.appendChild(userEl);
        });
    }
    
    async toggleVideo() {
        if (!this.localStream) {
            await this.startVideo();
        } else {
            this.stopVideo();
        }
    }
    
    async startVideo() {
        try {
            console.log('🎥 Запрашиваем доступ к экрану и микрофону...');
            
            // Запрашиваем скриншеринг
            const screenStream = await navigator.mediaDevices.getDisplayMedia({
                video: {
                    mediaSource: 'screen',
                    width: { ideal: 1920, max: 1920 },
                    height: { ideal: 1080, max: 1080 },
                    frameRate: { ideal: 30, max: 60 }
                },
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    sampleRate: 44100
                }
            });
            
            console.log('✅ Получен поток экрана:', screenStream.getTracks().map(t => t.kind));
            
            // Запрашиваем микрофон отдельно
            const micStream = await navigator.mediaDevices.getUserMedia({
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true
                },
                video: false
            });
            
            console.log('✅ Получен поток микрофона:', micStream.getTracks().map(t => t.kind));
            
            // Объединяем потоки
            this.localStream = new MediaStream([
                ...screenStream.getVideoTracks(),
                ...micStream.getAudioTracks()
            ]);
            
            console.log('✅ Объединенный поток создан:', this.localStream.getTracks().map(t => t.kind));
            
            this.localVideo.srcObject = this.localStream;
            this.videoArea.classList.add('active');
            this.videoToggle.classList.add('active');
            this.videoToggle.textContent = '🖥️ Остановить экран';
            
            // Обновляем статус пользователя
            this.broadcastingUsers.add(this.username);
            this.updateUsersList();
            
            // Обработчик остановки скриншеринга пользователем
            screenStream.getVideoTracks()[0].onended = () => {
                console.log('🛑 Скриншеринг остановлен пользователем');
                this.addSystemMessage('Демонстрация экрана завершена');
                this.stopVideo();
            };
            
            this.addSystemMessage('Демонстрация экрана включена. Кликните на пользователя для звонка.');
            
            // Сначала добавляем треки в существующие соединения
            this.addTracksToExistingConnections();

            // Затем звоним пользователям, с которыми нет соединения
            setTimeout(() => {
                this.onlineUsers.forEach(username => {
                    if (username !== this.username && !this.peerConnections.has(username)) {
                        console.log('🎥 Автоматический звонок новому пользователю:', username);
                        this.startVideoCall(username);
                    }
                });
            }, 1000); // Даем время на renegotiation
            
        } catch (error) {
            console.error('❌ Ошибка доступа к экрану/микрофону:', error);
            
            let errorMessage = 'Не удалось получить доступ: ';
            
            if (error.name === 'NotAllowedError') {
                errorMessage += 'Доступ запрещен. Разрешите доступ к экрану и микрофону.';
            } else if (error.name === 'NotFoundError') {
                errorMessage += 'Устройства не найдены.';
            } else if (error.name === 'NotSupportedError') {
                errorMessage += 'Браузер не поддерживает функцию.';
            } else {
                errorMessage += error.message;
            }
            
            alert(errorMessage);
            this.addSystemMessage(errorMessage);
        }
    }
    
    // Добавляем треки во все существующие подключения
    addTracksToExistingConnections() {
        if (!this.localStream) return;
        
        console.log('🔄 Добавляем треки в существующие соединения...');
        
        this.peerConnections.forEach((pc, username) => {
            console.log('➕ Добавляем новые треки для существующего соединения с', username);
            
            // Удаляем старые senders (если есть)
            const senders = pc.getSenders();
            senders.forEach(sender => {
                if (sender.track) {
                    console.log('🗑️ Удаляем старый sender:', sender.track.kind);
                    pc.removeTrack(sender);
                }
            });
            
            // Добавляем новые треки
            this.localStream.getTracks().forEach(track => {
                console.log('✅ Добавляем новый трек:', track.kind, 'для', username);
                pc.addTrack(track, this.localStream);
            });
            
            // Создаем новый offer после добавления треков
            this.renegotiateConnection(username, pc);
        });
    }
    
    // Перезапускаем соединение с новыми треками
    async renegotiateConnection(username, pc) {
        try {
            console.log('🔄 Перезапускаем соединение с', username);
            const offer = await pc.createOffer();
            await pc.setLocalDescription(offer);
            
            this.sendWebRTCMessage('webrtc_offer', username, {
                offer: offer
            });
            
            console.log('✅ Renegotiation offer отправлен для', username);
            
        } catch (error) {
            console.error('❌ Ошибка при перезапуске соединения с', username, ':', error);
        }
    }
    
    stopVideo() {
        console.log('🛑 Останавливаем видео...');
        
        if (this.localStream) {
            this.localStream.getTracks().forEach(track => {
                console.log('🛑 Останавливаем трек:', track.kind);
                track.stop();
            });
            this.localStream = null;
        }
        
        this.localVideo.srcObject = null;
        this.videoArea.classList.remove('active');
        this.videoToggle.classList.remove('active');
        this.videoToggle.textContent = '🖥️ Экран';
        
        // Обновляем статус пользователя
        this.broadcastingUsers.delete(this.username);
        this.updateUsersList();
        
        // Закрываем все WebRTC соединения
        this.peerConnections.forEach((pc, username) => {
            console.log('🔌 Закрываем WebRTC соединение с', username);
            pc.close();
        });
        this.peerConnections.clear();
        
        // Удаляем все удаленные видео
        const remoteVideos = this.videoArea.querySelectorAll('[id^="video-"]:not([id="localVideo"])');
        remoteVideos.forEach(video => {
            console.log('🗑️ Удаляем удаленное видео:', video.id);
            video.remove();
        });
        
        this.addSystemMessage('Демонстрация экрана остановлена');
    }
    
    toggleAudio() {
        if (this.localStream) {
            const audioTrack = this.localStream.getAudioTracks()[0];
            if (audioTrack) {
                audioTrack.enabled = !audioTrack.enabled;
                this.audioToggle.classList.toggle('active', audioTrack.enabled);
                this.audioToggle.textContent = audioTrack.enabled ? '🎤 Выключить микрофон' : '🎤 Включить микрофон';
                console.log('🎤 Аудио:', audioTrack.enabled ? 'включено' : 'выключено');
            }
        }
    }
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Запускаем чат при загрузке страницы
document.addEventListener('DOMContentLoaded', () => {
    console.log('🚀 Запуск Thoth Chat Client');
    new ThothChatClient();
});
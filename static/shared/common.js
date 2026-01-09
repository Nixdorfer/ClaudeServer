class WebSocketConnectionManager {
    constructor() {
        this.ws = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 3000;
        this.eventHandlers = {};
        this.statusIndicator = null;
        this.requestCallbacks = new Map();
        this.requestId = 0;
        this.isConnected = false;
        this.messageQueue = [];
        this.connectionPromise = null;
        this.connectionResolve = null;
        this.connectionReject = null;
        this.intentionalClose = false;
        this.heartbeatInterval = null;
        this.heartbeatDelay = 30000;
        this.isInitialized = false;
    }

    init() {
        if (this.isInitialized) {
            console.log('WebSocket管理器已初始化，跳过重复初始化');
            return this.connectionPromise || Promise.resolve();
        }
        this.isInitialized = true;

        this.createStatusIndicator();
        this.connect();
        return this.connectionPromise;
    }

    async waitForConnection(maxWaitTime = 30000) {
        if (this.isConnected) {
            return true;
        }

        if (!this.connectionPromise) {
            this.init();
        }

        const startTime = Date.now();

        while (Date.now() - startTime < maxWaitTime) {
            if (this.isConnected) {
                return true;
            }

            try {
                await this.connectionPromise;
                return true;
            } catch (error) {
                await new Promise(resolve => setTimeout(resolve, 500));

                if (this.isConnected) {
                    return true;
                }

                if (this.reconnectAttempts >= this.maxReconnectAttempts) {
                    console.error('WebSocket达到最大重连次数，放弃连接');
                    return false;
                }

            }
        }

        console.error('WebSocket连接等待超时');
        return false;
    }

    createStatusIndicator() {
        if (this.statusIndicator) {
            return;
        }

        const createIndicator = () => {
            const themeToggleContainer = document.querySelector('.theme-toggle-container');
            if (!themeToggleContainer) {
                setTimeout(createIndicator, 100);
                return;
            }

            const existing = document.getElementById('ws-status-indicator');
            if (existing) {
                this.statusIndicator = existing;
                return;
            }

            const container = document.createElement('div');
            container.className = 'ws-status-container';

            const label = document.createElement('div');
            label.className = 'ws-status-label';
            label.textContent = 'WebSocket';

            const indicator = document.createElement('div');
            indicator.id = 'ws-status-indicator';
            indicator.className = 'ws-status connecting';
            indicator.innerHTML = '<span class="status-dot"></span><span class="status-text">连接中...</span>';

            container.appendChild(label);
            container.appendChild(indicator);

            themeToggleContainer.parentNode.insertBefore(container, themeToggleContainer);

            this.statusIndicator = indicator;
        };

        createIndicator();
    }

    updateStatus(status, text) {
        if (!this.statusIndicator) return;
        this.statusIndicator.className = `ws-status ${status}`;
        this.statusIndicator.querySelector('.status-text').textContent = text;
    }

    connect() {
        this.connectionPromise = new Promise((resolve, reject) => {
            this.connectionResolve = resolve;
            this.connectionReject = reject;
        });

        this.intentionalClose = false;

        const lastConnected = sessionStorage.getItem('ws_last_connected');
        const now = Date.now();
        if (lastConnected && (now - parseInt(lastConnected)) < 5000) {
            this.updateStatus('connected', '已连接');
        } else {
            this.updateStatus('connecting', '连接中...');
        }

        if (this.ws) {
            this.intentionalClose = true;
            this.ws.close();
            this.intentionalClose = false;
        }

        try {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = `${protocol}//${window.location.host}/data/websocket/create`;

            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                console.log(`✓ WebSocket连接成功 [readyState=${this.ws.readyState}]`);
                this.isConnected = true;
                this.reconnectAttempts = 0;
                this.updateStatus('connected', '已连接');
                sessionStorage.setItem('ws_last_connected', Date.now().toString());

                if (this.connectionResolve) {
                    this.connectionResolve();
                    this.connectionResolve = null;
                    this.connectionReject = null;
                }

                this.startHeartbeat();

                this.flushMessageQueue();
            };

            this.ws.onclose = (event) => {
                const reason = event.reason || '无原因';
                const wasClean = event.wasClean ? '正常' : '异常';
                console.log(`✗ WebSocket连接关闭 [code=${event.code}, reason="${reason}", clean=${wasClean}, intentional=${this.intentionalClose}]`);
                this.isConnected = false;

                this.stopHeartbeat();

                if (!this.intentionalClose) {
                    this.handleConnectionError();
                }
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket 连接错误:', error);
                this.isConnected = false;
                if (this.connectionReject) {
                    this.connectionReject(error);
                }
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (error) {
                    console.error('解析 WebSocket 消息失败:', error);
                }
            };

        } catch (error) {
            console.error('创建 WebSocket 连接失败:', error);
            if (this.connectionReject) {
                this.connectionReject(error);
            }
            this.handleConnectionError();
        }
    }

    startHeartbeat() {
        this.stopHeartbeat();

        this.heartbeatInterval = setInterval(() => {
            if (this.isConnected && this.ws.readyState === WebSocket.OPEN) {
                this.send({
                    type: 'ping',
                    data: {}
                });
            }
        }, this.heartbeatDelay);
    }

    stopHeartbeat() {
        if (this.heartbeatInterval) {
            clearInterval(this.heartbeatInterval);
            this.heartbeatInterval = null;
        }
    }

    handleMessage(message) {
        const { type, data } = message;

        if (data && data.request_id !== undefined) {
            const callback = this.requestCallbacks.get(data.request_id);
            if (callback) {
                callback(data);
                if (type === 'response' || type === 'error') {
                    this.requestCallbacks.delete(data.request_id);
                }
            }
            return;
        }

        if (this.eventHandlers[type]) {
            this.eventHandlers[type]({ data: JSON.stringify(data) });
        }
    }

    flushMessageQueue() {
        while (this.messageQueue.length > 0 && this.isConnected) {
            const msg = this.messageQueue.shift();
            this.ws.send(JSON.stringify(msg));
        }
    }

    send(message) {
        if (this.isConnected && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        } else {
            this.messageQueue.push(message);
        }
    }

    async request(endpoint, method = 'GET', body = null) {
        return new Promise((resolve, reject) => {
            const requestId = ++this.requestId;
            const timeout = setTimeout(() => {
                this.requestCallbacks.delete(requestId);
                reject(new Error('Request timeout'));
            }, 30000);

            this.requestCallbacks.set(requestId, (data) => {
                clearTimeout(timeout);
                if (data.error) {
                    reject(new Error(data.error));
                } else {
                    resolve(data);
                }
            });

            this.send({
                type: 'api_request',
                data: {
                    request_id: requestId,
                    endpoint: endpoint,
                    method: method,
                    body: body
                }
            });
        });
    }

    handleConnectionError() {
        this.updateStatus('error', '连接失败');
        this.reconnectAttempts++;

        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.log('WebSocket重连失败，已达到最大重连次数');
            this.showReconnectDialog();
        } else {
            console.log(`${this.reconnectDelay / 1000} 秒后尝试重连 (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
            setTimeout(() => this.connect(), this.reconnectDelay);
        }
    }

    showReconnectDialog() {
        const dialog = document.createElement('div');
        dialog.className = 'ws-reconnect-dialog';
        dialog.innerHTML = `
            <div class="dialog-overlay"></div>
            <div class="dialog-content">
                <div class="dialog-icon">⚠️</div>
                <h3 class="dialog-title">连接失败</h3>
                <p class="dialog-message">WebSocket 实时连接失败，无法接收最新数据。<br>建议刷新页面重新连接。</p>
                <div class="dialog-actions">
                    <button class="btn-cancel" onclick="this.closest('.ws-reconnect-dialog').remove(); wsManager.updateStatus('error', '连接失败 (已停止重试)');">稍后再试</button>
                    <button class="btn-confirm" onclick="window.location.reload();">刷新页面</button>
                </div>
            </div>
        `;
        document.body.appendChild(dialog);
    }

    on(eventName, handler) {
        this.eventHandlers[eventName] = handler;
    }

    close() {
        this.intentionalClose = true;
        this.stopHeartbeat();
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.isConnected = false;
        this.updateStatus('disconnected', '已断开');
    }
}

const wsManager = new WebSocketConnectionManager();
const sseManager = wsManager;

function toggleTheme() {
    const theme = document.getElementById('themeToggle').checked ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('theme', theme);
}

function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'dark';
    document.documentElement.setAttribute('data-theme', savedTheme);
    const toggle = document.getElementById('themeToggle');
    if (toggle) toggle.checked = savedTheme === 'dark';
}

async function loadVersion() {
    try {
        const cachedVersion = localStorage.getItem('app_version');
        if (cachedVersion) {
            document.querySelectorAll('.version').forEach(el => {
                el.textContent = 'v' + cachedVersion;
            });
        }

        const config = await fetchConfig();
        if (config && config.version) {
            localStorage.setItem('app_version', config.version);
            document.querySelectorAll('.version').forEach(el => {
                el.textContent = 'v' + config.version;
            });
        }
    } catch (error) {
        console.error('加载版本失败:', error);
    }
}

window.addEventListener('DOMContentLoaded', () => {
    initTheme();
    loadVersion();
});

window.addEventListener('load', () => {
    setTimeout(() => {
        if (!wsManager.isInitialized && !wsManager.isConnected) {
            console.log('页面加载完成，初始化WebSocket连接...');
            wsManager.init().catch(error => {
                console.log('WebSocket初始连接失败，等待重连...');
            });
        }
    }, 500);
});

window.addEventListener('beforeunload', () => {
    if (wsManager.isConnected) {
        wsManager.close();
    }
});

function formatTime(timeStr) {
    const date = new Date(timeStr);
    return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
    });
}

function formatDuration(seconds) {
    if (seconds < 60) return seconds.toFixed(2) + 's';
    const mins = Math.floor(seconds / 60);
    const secs = (seconds % 60).toFixed(0);
    return `${mins}m ${secs}s`;
}

function copyText(text, event) {
    const btn = event.target;
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
        document.execCommand('copy');
        const originalText = btn.textContent;
        btn.textContent = '已复制';
        setTimeout(() => {
            btn.textContent = originalText;
        }, 1500);
    } catch (err) {
        alert('复制失败');
    }
    document.body.removeChild(textarea);
}

async function fetchStats() {
    const connected = await wsManager.waitForConnection();
    if (!connected) {
        console.error('WebSocket连接失败，无法获取统计信息');
        return null;
    }
    try {
        return await wsManager.request('/api/stats', 'GET');
    } catch (error) {
        console.error('获取统计失败:', error);
        return null;
    }
}

async function fetchConfig() {
    const connected = await wsManager.waitForConnection();
    if (!connected) {
        console.error('WebSocket连接失败，无法获取配置');
        return null;
    }
    try {
        return await wsManager.request('/api/config', 'GET');
    } catch (error) {
        console.error('获取配置失败:', error);
        return null;
    }
}

async function fetchUsage() {
    const connected = await wsManager.waitForConnection();
    if (!connected) {
        console.error('WebSocket连接失败，无法获取使用量');
        return null;
    }
    try {
        return await wsManager.request('/api/usage', 'GET');
    } catch (error) {
        console.error('获取使用量失败:', error);
        return null;
    }
}

async function apiRequest(endpoint, method = 'GET', body = null) {
    const connected = await wsManager.waitForConnection();
    if (!connected) {
        console.error('WebSocket连接失败，无法发送请求:', endpoint);
        throw new Error('WebSocket连接失败，请刷新页面');
    }
    try {
        return await wsManager.request(endpoint, method, body);
    } catch (error) {
        console.error(`API请求失败 (${endpoint}):`, error);
        throw error;
    }
}

function updateStatsDisplay(data) {
    if (!data) return;

    const elements = {
        processing: document.getElementById('processing'),
        completed: document.getElementById('completed'),
        failed: document.getElementById('failed'),
        tpm: document.getElementById('tpm'),
        rpm: document.getElementById('rpm'),
        rpd: document.getElementById('rpd')
    };

    if (elements.processing) elements.processing.textContent = data.processing;
    if (elements.completed) elements.completed.textContent = data.completed;
    if (elements.failed) elements.failed.textContent = data.failed;

    const tpmValue = Math.round(data.tpm);
    const rpmValue = Math.round(data.rpm);
    const rpdValue = Math.round(data.rpd);

    if (elements.tpm) {
        elements.tpm.textContent = tpmValue;
        elements.tpm.style.color = tpmValue > 10000 ? '#f44336' : '#4CAF50';
    }

    if (elements.rpm) {
        elements.rpm.textContent = rpmValue;
        elements.rpm.style.color = rpmValue > 30 ? '#f44336' : '#4CAF50';
    }

    if (elements.rpd) {
        elements.rpd.textContent = rpdValue;
        elements.rpd.style.color = rpdValue > 2000 ? '#f44336' : '#4CAF50';
    }

    const banner = document.getElementById('warningBanner');
    const message = document.getElementById('warningMessage');
    if (data.service_shutdown && banner && message) {
        message.textContent = data.shutdown_reason || '服务已停止';
        banner.style.display = 'block';
    } else if (banner) {
        banner.style.display = 'none';
    }
}

document.addEventListener('DOMContentLoaded', function() {
    const currentPage = window.location.pathname;
    document.querySelectorAll('.nav-link').forEach(link => {
        if (link.getAttribute('href') === currentPage) {
            link.classList.add('active');
        }
    });

    const modalContents = document.querySelectorAll('.modal-content');
    modalContents.forEach(content => {
        content.onclick = function(e) {
            e.stopPropagation();
        };
    });

    window.onclick = function(event) {
        if (event.target.classList.contains('modal')) {
            event.target.style.display = 'none';
            document.body.style.overflow = 'auto';
        }
    };
});

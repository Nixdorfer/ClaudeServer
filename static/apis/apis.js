const apiData = {
    dialogue: {
        title: '对话接口',
        intro: '提供三种对话方式：HTTP、SSE、WebSocket',
        apis: [
            {
                method: 'POST',
                path: '/chat/dialogue/http',
                description: 'HTTP对话请求(会话保持，长超时)',
                fullPath: 'http://localhost:5000/chat/dialogue/http',
                request: {
                    "conversation_id": "optional-conversation-id",
                    "request": "Hello!",
                    "model": "claude-sonnet-4-20250514",
                    "style": "normal"
                },
                response: {
                    "conversation_id": "conv-abc123",
                    "response": "Hello! How can I help you?"
                }
            },
            {
                method: 'GET',
                path: '/chat/dialogue/event',
                description: 'SSE流式对话(查询参数传递)',
                fullPath: 'http://localhost:5000/chat/dialogue/event?request=Hello&conversation_id=optional&model=claude-sonnet-4-20250514',
                request: null,
                response: {
                    "event": "conversation_id",
                    "data": {"conversation_id": "conv-abc123"}
                },
                notes: '返回多个SSE事件：conversation_id, content, done, error'
            },
            {
                method: 'GET',
                path: '/chat/dialogue/websocket',
                description: 'WebSocket流式对话(单次连接)',
                fullPath: 'ws://localhost:5000/chat/dialogue/websocket',
                request: {
                    "request": "Hello!",
                    "conversation_id": "optional-conversation-id",
                    "model": "claude-sonnet-4-20250514",
                    "style": "normal"
                },
                response: {
                    "type": "conversation_id",
                    "data": {"conversation_id": "conv-abc123"}
                },
                notes: '返回多个WebSocket消息：conversation_id, content, done, error'
            },
            {
                method: 'POST',
                path: '/chat/dialogue/keepalive/:id',
                description: '保持对话会话活跃',
                fullPath: 'http://localhost:5000/chat/dialogue/keepalive/{conversation_id}',
                request: null,
                response: {
                    "message": "Session refreshed"
                }
            },
            {
                method: 'DELETE',
                path: '/chat/dialogue/:id',
                description: '删除对话会话',
                fullPath: 'http://localhost:5000/chat/dialogue/{conversation_id}',
                request: null,
                response: {
                    "message": "Dialogue deleted successfully"
                }
            }
        ]
    },
    websocket: {
        title: '持久化WebSocket',
        intro: '建立持久连接，支持多种消息类型和API请求',
        apis: [
            {
                method: 'GET',
                path: '/data/websocket/create',
                description: '创建持久WebSocket连接',
                fullPath: 'ws://localhost:5000/data/websocket/create',
                request: null,
                response: {
                    "type": "connected",
                    "data": {
                        "status": "connected",
                        "message": "WebSocket connection established"
                    }
                },
                notes: '连接建立后可发送多种类型消息'
            },
            {
                method: 'WS',
                path: '消息类型: dialogue',
                description: '通过持久连接发送对话请求',
                fullPath: 'ws://localhost:5000/data/websocket/create',
                request: {
                    "type": "dialogue",
                    "data": {
                        "request": "Hello!",
                        "conversation_id": "optional-conversation-id",
                        "model": "claude-sonnet-4-20250514",
                        "style": "normal"
                    }
                },
                response: {
                    "type": "content",
                    "data": {"delta": "Hello!", "text": "Hello!"}
                }
            },
            {
                method: 'WS',
                path: '消息类型: api_request',
                description: '通过持久连接发送API请求',
                fullPath: 'ws://localhost:5000/data/websocket/create',
                request: {
                    "type": "api_request",
                    "data": {
                        "request_id": 1,
                        "endpoint": "/api/stats",
                        "method": "GET"
                    }
                },
                response: {
                    "type": "response",
                    "data": {
                        "request_id": 1,
                        "processing": 0,
                        "completed": 100,
                        "failed": 5
                    }
                }
            },
            {
                method: 'WS',
                path: '消息类型: keepalive',
                description: '保持会话活跃',
                fullPath: 'ws://localhost:5000/data/websocket/create',
                request: {
                    "type": "keepalive",
                    "data": {
                        "conversation_id": "conv-abc123"
                    }
                },
                response: {
                    "type": "keepalive",
                    "data": {
                        "conversation_id": "conv-abc123",
                        "status": "keepalive",
                        "message": "Session refreshed"
                    }
                }
            },
            {
                method: 'WS',
                path: '消息类型: ping',
                description: '心跳检测',
                fullPath: 'ws://localhost:5000/data/websocket/create',
                request: {
                    "type": "ping",
                    "data": {}
                },
                response: {
                    "type": "pong",
                    "data": {"timestamp": "2025-11-03T12:00:00Z"}
                }
            }
        ]
    }
};

function toggleApiMenu() {
    const subMenu = document.getElementById('apiSubMenu');
    const icon = document.querySelector('.collapse-icon');
    if (!subMenu || !icon) return;

    if (subMenu.classList.contains('open')) {
        subMenu.classList.remove('open');
        icon.textContent = '▶';
    } else {
        subMenu.classList.add('open');
        icon.textContent = '▼';
    }
}

function showCategory(category, clickedElement) {
    document.querySelectorAll('.sub-nav-item').forEach(item => item.classList.remove('active'));
    if (clickedElement) {
        clickedElement.classList.add('active');
    } else {
        const items = document.querySelectorAll('.sub-nav-item');
        items.forEach(item => {
            const link = item.querySelector('.sub-nav-link');
            if (link && link.textContent.trim() === getCategoryName(category)) {
                item.classList.add('active');
            }
        });
    }

    const data = apiData[category];
    const introContainer = document.getElementById('apiIntro');
    const container = document.getElementById('apiCards');

    if (!container) return;

    if (introContainer) {
        introContainer.innerHTML = `
            <h2>${data.title}</h2>
            <p>${data.intro}</p>
        `;
    }

    container.innerHTML = '';
    data.apis.forEach((api, index) => {
        const card = document.createElement('div');
        card.className = 'api-card-compact';
        card.onclick = () => showApiDetail(api);

        card.innerHTML = `
            <div class="api-card-header-compact">
                <span class="method ${api.method.toLowerCase()}">${api.method}</span>
            </div>
            <div class="api-card-body">
                <div class="api-path-compact">${api.path}</div>
                <p class="api-description-compact">${api.description}</p>
            </div>
        `;
        container.appendChild(card);
    });
}

function showApiDetail(api) {
    const modal = document.getElementById('apiDetailModal');
    if (modal) modal.style.display = 'flex';
    document.body.style.overflow = 'hidden';

    const modalTitle = document.getElementById('apiModalTitle');
    const detailMethod = document.getElementById('apiDetailMethod');
    const detailPath = document.getElementById('apiDetailPath');
    const detailFullPath = document.getElementById('apiDetailFullPath');
    const detailRequest = document.getElementById('apiDetailRequest');
    const detailResponse = document.getElementById('apiDetailResponse');
    const detailNotes = document.getElementById('apiDetailNotes');
    const requestSection = document.getElementById('apiDetailRequestSection');
    const notesSection = document.getElementById('apiDetailNotesSection');

    if (modalTitle) modalTitle.textContent = api.description || 'API详情';

    if (detailMethod) {
        detailMethod.innerHTML = `<span class="method ${api.method.toLowerCase()}">${api.method}</span>`;
    }

    if (detailPath) {
        detailPath.innerHTML = `<code>${api.path}</code>`;
    }

    if (detailFullPath) {
        detailFullPath.textContent = api.fullPath || '-';
    }

    if (detailRequest && requestSection) {
        if (api.request) {
            detailRequest.textContent = JSON.stringify(api.request, null, 2);
            requestSection.style.display = 'block';
        } else {
            detailRequest.textContent = '无需请求体';
            requestSection.style.display = 'block';
        }
    }

    if (detailResponse) {
        detailResponse.textContent = JSON.stringify(api.response, null, 2);
    }

    if (detailNotes && notesSection) {
        if (api.notes) {
            detailNotes.textContent = api.notes;
            notesSection.style.display = 'block';
        } else {
            notesSection.style.display = 'none';
        }
    }
}

function closeApiModal() {
    const modal = document.getElementById('apiDetailModal');
    if (modal) modal.style.display = 'none';
    document.body.style.overflow = 'auto';
}

function getCategoryName(category) {
    const names = {
        'dialogue': '对话接口',
        'websocket': '持久化WebSocket'
    };
    return names[category] || category;
}

window.addEventListener('DOMContentLoaded', () => {
    const subMenu = document.getElementById('apiSubMenu');
    if (subMenu) {
        subMenu.classList.add('open');
    }
    showCategory('dialogue');
});

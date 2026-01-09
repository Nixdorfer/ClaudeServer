let dataUpdateInterval = null;
let currentDialogueId = null;
let streamUpdateInterval = null;

async function fetchAndUpdateStats() {
    try {
        const stats = await fetchStats();
        if (stats) {
            updateStatsDisplay(stats);
        }
    } catch (error) {
        console.error('获取统计数据失败:', error);
    }
}

async function fetchAndUpdateDialogues() {
    try {
        const data = await apiRequest('/api/dialogues', 'GET');
        if (data && data.dialogues) {
            updateDialoguesTable(data.dialogues);
        }
    } catch (error) {
        console.error('获取对话列表失败:', error);
    }
}

function updateDialoguesTable(dialogues) {
    const table = document.getElementById('dialoguesTable');
    const emptyState = document.getElementById('emptyDialogues');

    if (!table) return;

    table.innerHTML = '';

    if (!dialogues || dialogues.length === 0) {
        if (emptyState) emptyState.style.display = 'block';
        return;
    }

    if (emptyState) emptyState.style.display = 'none';

    dialogues.forEach(dialogue => {
        const row = document.createElement('tr');

        const conversationId = dialogue.conversation_id || '-';
        const lastUsed = dialogue.last_used ? formatTime(dialogue.last_used) : '-';
        const relativeTime = dialogue.last_used ? getRelativeTime(dialogue.last_used) : '-';
        const idleTime = dialogue.idle_seconds ? formatDuration(dialogue.idle_seconds) : '-';

        row.innerHTML = `
            <td><code class="conversation-id" title="${escapeHtml(conversationId)}">${escapeHtml(conversationId.substring(0, 16))}...</code></td>
            <td>${lastUsed}</td>
            <td>${relativeTime}</td>
            <td>${idleTime}</td>
            <td>
                <button class="btn btn-view" onclick="viewDialogueStream('${escapeHtml(conversationId)}')">查看</button>
                <button class="btn btn-delete" onclick="deleteDialogue('${escapeHtml(conversationId)}')">删除</button>
            </td>
        `;
        table.appendChild(row);
    });
}

function getRelativeTime(timeStr) {
    if (!timeStr) return '-';
    const date = new Date(timeStr);
    const now = new Date();
    const diffMs = now - date;
    const diffSecs = Math.floor(diffMs / 1000);

    if (diffSecs < 60) return `${diffSecs}秒前`;
    if (diffSecs < 3600) return `${Math.floor(diffSecs / 60)}分钟前`;
    if (diffSecs < 86400) return `${Math.floor(diffSecs / 3600)}小时前`;
    return `${Math.floor(diffSecs / 86400)}天前`;
}

async function viewDialogueStream(conversationId) {
    currentDialogueId = conversationId;

    const modal = document.getElementById('streamModal');
    const qaList = document.getElementById('qaList');
    const streamMessage = document.getElementById('streamMessage');
    const streamResponse = document.getElementById('streamResponse');

    if (modal) modal.style.display = 'flex';
    document.body.style.overflow = 'hidden';

    if (qaList) qaList.innerHTML = '<div class="loading">加载中...</div>';
    if (streamMessage) streamMessage.textContent = '';
    if (streamResponse) streamResponse.textContent = '等待选择问答...';

    try {
        const data = await apiRequest(`/api/dialogues/${conversationId}/history`, 'GET');
        if (data && data.messages) {
            updateQAList(data.messages);

            if (data.messages.length > 0) {
                selectQA(data.messages.length - 1, data.messages);
            }
        } else {
            if (qaList) qaList.innerHTML = '<div class="no-data">暂无消息记录</div>';
        }
    } catch (error) {
        console.error('加载对话历史失败:', error);
        if (qaList) qaList.innerHTML = '<div class="error">加载失败</div>';
    }

    startStreamUpdate();
}

function updateQAList(messages) {
    const qaList = document.getElementById('qaList');
    if (!qaList) return;

    qaList.innerHTML = '';

    messages.forEach((msg, index) => {
        const item = document.createElement('div');
        item.className = 'qa-item';
        item.onclick = () => selectQA(index, messages);

        const status = translateStatus(msg.status);
        const statusClass = msg.status === 'done' ? 'status-done' :
                           msg.status === 'failed' ? 'status-failed' :
                           'status-processing';

        const preview = msg.request ? truncateString(msg.request, 50) : '(无内容)';

        item.innerHTML = `
            <div class="qa-item-header">
                <span class="qa-number">#${msg.exchange_number || index + 1}</span>
                <span class="qa-status ${statusClass}">${status}</span>
            </div>
            <div class="qa-preview">${escapeHtml(preview)}</div>
        `;
        qaList.appendChild(item);
    });
}

function selectQA(index, messages) {
    const msg = messages[index];
    if (!msg) return;

    const streamMessage = document.getElementById('streamMessage');
    const streamResponse = document.getElementById('streamResponse');

    document.querySelectorAll('.qa-item').forEach((item, i) => {
        if (i === index) {
            item.classList.add('active');
        } else {
            item.classList.remove('active');
        }
    });

    if (streamMessage) {
        streamMessage.textContent = msg.request || '(无内容)';
    }

    if (streamResponse) {
        if (msg.response) {
            streamResponse.textContent = msg.response;
            streamResponse.classList.remove('loading');
        } else if (msg.status === 'processing') {
            streamResponse.textContent = '处理中...';
            streamResponse.classList.add('loading');
        } else {
            streamResponse.textContent = '(无响应)';
            streamResponse.classList.remove('loading');
        }
    }
}

function startStreamUpdate() {
    if (streamUpdateInterval) {
        clearInterval(streamUpdateInterval);
    }

    streamUpdateInterval = setInterval(async () => {
        if (!currentDialogueId) return;

        try {
            const data = await apiRequest(`/api/dialogues/${currentDialogueId}/history`, 'GET');
            if (data && data.messages) {
                updateQAList(data.messages);

                const activeItem = document.querySelector('.qa-item.active');
                if (activeItem) {
                    const activeIndex = Array.from(activeItem.parentNode.children).indexOf(activeItem);
                    if (activeIndex >= 0 && activeIndex < data.messages.length) {
                        selectQA(activeIndex, data.messages);
                    }
                }
            }
        } catch (error) {
            console.error('更新对话详情失败:', error);
        }
    }, 2000);
}

function stopStreamUpdate() {
    if (streamUpdateInterval) {
        clearInterval(streamUpdateInterval);
        streamUpdateInterval = null;
    }
}

function closeStreamModal() {
    const modal = document.getElementById('streamModal');
    if (modal) modal.style.display = 'none';
    document.body.style.overflow = 'auto';

    currentDialogueId = null;
    stopStreamUpdate();
}

async function deleteDialogue(conversationId) {
    if (!confirm('确定要删除这个对话吗？')) {
        return;
    }

    try {
        await apiRequest(`/api/dialogues/${conversationId}`, 'DELETE');
        console.log('对话删除成功');

        await fetchAndUpdateDialogues();

        if (currentDialogueId === conversationId) {
            closeStreamModal();
        }
    } catch (error) {
        console.error('删除对话失败:', error);
        alert('删除对话失败: ' + error.message);
    }
}

async function clearDialogueCache() {
    if (!confirm('确定要清除所有对话缓存吗？此操作不可恢复！')) {
        return;
    }

    try {
        const data = await apiRequest('/api/dialogues', 'GET');
        if (data && data.dialogues && data.dialogues.length > 0) {
            for (const dialogue of data.dialogues) {
                await apiRequest(`/api/dialogues/${dialogue.conversation_id}`, 'DELETE');
            }
            console.log('所有对话已清除');
            await fetchAndUpdateDialogues();
        } else {
            alert('没有对话需要清除');
        }
    } catch (error) {
        console.error('清除缓存失败:', error);
        alert('清除缓存失败: ' + error.message);
    }
}

function startDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
    }

    fetchAndUpdateStats();
    fetchAndUpdateDialogues();

    dataUpdateInterval = setInterval(() => {
        fetchAndUpdateStats();
        fetchAndUpdateDialogues();
    }, 5000);
}

function stopDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
        dataUpdateInterval = null;
    }
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function truncateString(str, maxLength) {
    if (!str) return '';
    if (str.length <= maxLength) return str;
    return str.substring(0, maxLength) + '...';
}

function translateStatus(status) {
    const statusMap = {
        'done': '已完成',
        'processing': '处理中',
        'failed': '失败',
        'overloaded': '过载'
    };
    return statusMap[status] || status;
}

document.addEventListener('DOMContentLoaded', () => {
});

window.addEventListener('load', () => {
    setTimeout(() => {
        startDataUpdates();
    }, 1000);
});

window.addEventListener('beforeunload', () => {
    stopDataUpdates();
    stopStreamUpdate();
});

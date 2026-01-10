let currentPage = 1;
let pageSize = 20;
let totalRecords = 0;
let allRecords = [];
let filteredRecords = [];
let searchTerm = '';
let dataUpdateInterval = null;
let isPrivateMode = false;

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

async function fetchAndUpdateHistory() {
    try {
        const data = await apiRequest('/api/records', 'POST', {
            limit: 1000
        });
        if (data && data.messages) {
            allRecords = data.messages;
            if (data.private_mode !== undefined) {
                isPrivateMode = data.private_mode;
                applyPrivateModeUI();
            }
            applySearchFilter();
            updateHistoryTable();
        }
    } catch (error) {
        console.error('获取历史记录失败:', error);
    }
}

function applyPrivateModeUI() {
    const idColumnHeader = document.getElementById('idColumnHeader');
    const actionColumnHeader = document.getElementById('actionColumnHeader');
    if (isPrivateMode) {
        if (idColumnHeader) idColumnHeader.textContent = '设备ID';
        if (actionColumnHeader) actionColumnHeader.style.display = 'none';
    } else {
        if (idColumnHeader) idColumnHeader.textContent = '对话ID';
        if (actionColumnHeader) actionColumnHeader.style.display = '';
    }
}

function applySearchFilter() {
    if (!searchTerm) {
        filteredRecords = allRecords;
    } else {
        const term = searchTerm.toLowerCase();
        filteredRecords = allRecords.filter(record => {
            const conversationId = (record.conversation_id || '').toLowerCase();
            const request = (record.request || '').toLowerCase();
            const response = (record.response || '').toLowerCase();
            const id = String(record.id || '').toLowerCase();

            return conversationId.includes(term) ||
                   request.includes(term) ||
                   response.includes(term) ||
                   id.includes(term);
        });
    }

    totalRecords = filteredRecords.length;
    currentPage = 1;
}

function updateHistoryTable() {
    const table = document.getElementById('historyTable');
    const emptyState = document.getElementById('emptyHistory');

    if (!table) return;

    table.innerHTML = '';

    if (!filteredRecords || filteredRecords.length === 0) {
        if (emptyState) emptyState.style.display = 'block';
        updatePagination();
        return;
    }

    if (emptyState) emptyState.style.display = 'none';

    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = Math.min(startIndex + pageSize, filteredRecords.length);
    const pageRecords = filteredRecords.slice(startIndex, endIndex);

    pageRecords.forEach(record => {
        const row = document.createElement('tr');
        const id = record.id || '-';
        const receiveTime = record.receive_time ? formatTime(record.receive_time) : '-';
        const duration = record.duration ? record.duration.toFixed(2) : '-';
        const tokens = record.tokens || '-';
        const status = translateStatus(record.status);
        const statusClass = getStatusClass(record.status);
        const preview = record.request ? truncateString(record.request, 60) : '(无内容)';
        if (isPrivateMode) {
            const deviceId = record.device_id || '-';
            row.innerHTML = `
                <td>${id}</td>
                <td><code class="conversation-id">${deviceId}</code></td>
                <td>${receiveTime}</td>
                <td>${duration}</td>
                <td>${tokens}</td>
                <td><span class="status-badge ${statusClass}">${status}</span></td>
                <td class="preview-cell">${escapeHtml(preview)}</td>
                <td style="display:none;"></td>
            `;
        } else {
            const conversationId = record.conversation_id || '-';
            row.innerHTML = `
                <td>${id}</td>
                <td><code class="conversation-id" title="${escapeHtml(conversationId)}">${escapeHtml(conversationId.substring(0, 12))}...</code></td>
                <td>${receiveTime}</td>
                <td>${duration}</td>
                <td>${tokens}</td>
                <td><span class="status-badge ${statusClass}">${status}</span></td>
                <td class="preview-cell">${escapeHtml(preview)}</td>
                <td>
                    <button class="btn btn-view" onclick="viewRecordDetail(${id})">查看详情</button>
                </td>
            `;
        }
        table.appendChild(row);
    });

    updatePagination();
}

function updatePagination() {
    const totalPages = Math.max(1, Math.ceil(totalRecords / pageSize));
    const currentPageSpan = document.getElementById('currentPageSpan');
    const totalPagesSpan = document.getElementById('totalPagesSpan');
    const prevBtn = document.getElementById('prevBtn');
    const nextBtn = document.getElementById('nextBtn');
    const pageInput = document.getElementById('pageInput');

    if (currentPageSpan) currentPageSpan.textContent = currentPage;
    if (totalPagesSpan) totalPagesSpan.textContent = totalPages;
    if (pageInput) pageInput.value = currentPage;

    if (prevBtn) {
        prevBtn.disabled = currentPage <= 1;
    }

    if (nextBtn) {
        nextBtn.disabled = currentPage >= totalPages;
    }
}

function changePage(newPage) {
    const totalPages = Math.ceil(totalRecords / pageSize);

    if (newPage < 1 || newPage > totalPages) {
        return;
    }

    currentPage = newPage;
    updateHistoryTable();
}

function jumpToPage() {
    const pageInput = document.getElementById('pageInput');
    if (!pageInput) return;

    const page = parseInt(pageInput.value);
    if (isNaN(page)) {
        alert('请输入有效的页码');
        return;
    }

    changePage(page);
}

function searchHistory() {
    const searchInput = document.getElementById('searchInput');
    if (!searchInput) return;

    searchTerm = searchInput.value.trim();
    applySearchFilter();
    updateHistoryTable();
}

function clearSearch() {
    const searchInput = document.getElementById('searchInput');
    if (searchInput) {
        searchInput.value = '';
    }

    searchTerm = '';
    applySearchFilter();
    updateHistoryTable();
}

async function viewRecordDetail(id) {
    const modal = document.getElementById('detailModal');
    if (modal) modal.style.display = 'flex';
    document.body.style.overflow = 'hidden';

    const fields = ['detailId', 'detailConversationId', 'detailReceiveTime',
                   'detailDuration', 'detailTokens', 'detailStatus',
                   'detailRequest', 'detailResponse'];

    fields.forEach(fieldId => {
        const element = document.getElementById(fieldId);
        if (element) element.textContent = '加载中...';
    });

    try {
        const record = await apiRequest(`/api/record/${id}`, 'GET');

        if (!record) {
            alert('记录不存在');
            closeModal();
            return;
        }

        const detailId = document.getElementById('detailId');
        const detailConversationId = document.getElementById('detailConversationId');
        const detailReceiveTime = document.getElementById('detailReceiveTime');
        const detailDuration = document.getElementById('detailDuration');
        const detailTokens = document.getElementById('detailTokens');
        const detailStatus = document.getElementById('detailStatus');
        const detailRequest = document.getElementById('detailRequest');
        const detailResponse = document.getElementById('detailResponse');

        if (detailId) detailId.textContent = record.id || '-';
        if (detailConversationId) detailConversationId.textContent = record.conversation_id || '-';
        if (detailReceiveTime) detailReceiveTime.textContent = record.receive_time ? formatTime(record.receive_time) : '-';

        let durationText = '-';
        if (record.duration) {
            durationText = `${record.duration.toFixed(2)}s`;
        }
        if (detailDuration) detailDuration.textContent = durationText;

        let tokensText = '';
        if (record.request_tokens) tokensText += `请求: ${record.request_tokens}\n`;
        if (record.response_tokens) tokensText += `响应: ${record.response_tokens}\n`;
        if (record.tokens) tokensText += `总计: ${record.tokens}`;
        if (detailTokens) detailTokens.textContent = tokensText || '-';

        if (detailStatus) {
            const statusText = translateStatus(record.status);
            const statusClass = getStatusClass(record.status);
            detailStatus.innerHTML = `<span class="status-badge ${statusClass}">${statusText}</span>`;
        }

        if (detailRequest) detailRequest.textContent = record.request || '(无内容)';
        if (detailResponse) detailResponse.textContent = record.response || '(无响应)';

    } catch (error) {
        console.error('获取记录详情失败:', error);
        alert('获取记录详情失败: ' + error.message);
        closeModal();
    }
}

function closeModal() {
    const modal = document.getElementById('detailModal');
    if (modal) modal.style.display = 'none';
    document.body.style.overflow = 'auto';
}

async function clearCache() {
    if (!confirm('确定要清除所有历史记录吗？此操作不可恢复！')) {
        return;
    }

    alert('清除缓存功能需要后端支持，暂未实现');
}

function startDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
    }

    fetchAndUpdateStats();
    fetchAndUpdateHistory();

    dataUpdateInterval = setInterval(() => {
        fetchAndUpdateStats();
        fetchAndUpdateHistory();
    }, 10000);
}

function stopDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
        dataUpdateInterval = null;
    }
}

function formatTime(timeStr) {
    if (!timeStr) return '-';
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

function translateStatus(status) {
    const statusMap = {
        'done': '已完成',
        'processing': '处理中',
        'failed': '失败',
        'overloaded': '过载'
    };
    return statusMap[status] || status;
}

function getStatusClass(status) {
    const classMap = {
        'done': 'status-done',
        'processing': 'status-processing',
        'failed': 'status-failed',
        'overloaded': 'status-overloaded'
    };
    return classMap[status] || '';
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

document.addEventListener('DOMContentLoaded', () => {
    const searchInput = document.getElementById('searchInput');
    if (searchInput) {
        searchInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                searchHistory();
            }
        });
    }

    const pageInput = document.getElementById('pageInput');
    if (pageInput) {
        pageInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                jumpToPage();
            }
        });
    }
});

window.addEventListener('load', () => {
    setTimeout(() => {
        startDataUpdates();
    }, 1000);
});

window.addEventListener('beforeunload', () => {
    stopDataUpdates();
});

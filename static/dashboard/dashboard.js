let tokenChart = null;
let successChart = null;
let currentTimeRange = '1h';
let dataUpdateInterval = null;

async function fetchAndUpdateStats() {
    try {
        const stats = await fetchStats();
        if (stats) {
            updateStatsDisplay(stats);
            updateSuccessChart(stats);
        }
    } catch (error) {
        console.error('获取统计数据失败:', error);
    }
}

async function fetchAndUpdateUsage() {
    try {
        const usage = await fetchUsage();
        if (usage) {
            updateUsageDisplay(usage);
        }
    } catch (error) {
        console.error('获取使用量数据失败:', error);
    }
}

function startDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
    }

    fetchAndUpdateStats();
    fetchAndUpdateUsage();

    dataUpdateInterval = setInterval(() => {
        fetchAndUpdateStats();
        fetchAndUpdateUsage();
    }, 5000);
}

function stopDataUpdates() {
    if (dataUpdateInterval) {
        clearInterval(dataUpdateInterval);
        dataUpdateInterval = null;
    }
}

function updateUsageDisplay(usage) {
    const fiveHourValue = usage.five_hour_utilization || 0;
    const fiveHourProgress = document.getElementById('fiveHourProgress');
    const fiveHourValueEl = document.getElementById('fiveHourValue');
    const fiveHourReset = document.getElementById('fiveHourReset');

    if (fiveHourProgress) fiveHourProgress.style.width = fiveHourValue + '%';
    if (fiveHourValueEl) fiveHourValueEl.textContent = fiveHourValue + '%';
    if (fiveHourReset && usage.five_hour_resets_at) {
        fiveHourReset.textContent = '重置于 ' + formatResetTime(usage.five_hour_resets_at);
    }

    const sevenDayValue = usage.seven_day_utilization || 0;
    const sevenDayProgress = document.getElementById('sevenDayProgress');
    const sevenDayValueEl = document.getElementById('sevenDayValue');
    const sevenDayReset = document.getElementById('sevenDayReset');

    if (sevenDayProgress) sevenDayProgress.style.width = sevenDayValue + '%';
    if (sevenDayValueEl) sevenDayValueEl.textContent = sevenDayValue + '%';
    if (sevenDayReset && usage.seven_day_resets_at) {
        sevenDayReset.textContent = '重置于 ' + formatResetTime(usage.seven_day_resets_at);
    }

    const sevenDayOpusValue = usage.seven_day_opus_utilization || 0;
    const sevenDayOpusProgress = document.getElementById('sevenDayOpusProgress');
    const sevenDayOpusValueEl = document.getElementById('sevenDayOpusValue');
    const sevenDayOpusReset = document.getElementById('sevenDayOpusReset');

    if (sevenDayOpusProgress) sevenDayOpusProgress.style.width = sevenDayOpusValue + '%';
    if (sevenDayOpusValueEl) sevenDayOpusValueEl.textContent = sevenDayOpusValue + '%';
    if (sevenDayOpusReset && usage.seven_day_opus_resets_at) {
        sevenDayOpusReset.textContent = '重置于 ' + formatResetTime(usage.seven_day_opus_resets_at);
    }
}

function formatResetTime(timeStr) {
    if (!timeStr) return '--';
    const date = new Date(timeStr);
    return date.toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

async function loadConfig() {
    try {
        const config = await fetchConfig();
        if (!config) return;

        const configDisplay = document.getElementById('configDisplay');
        if (!configDisplay) return;

        configDisplay.innerHTML = '<tr><td>最大TPM</td><td>' + (config.max_tpm || '无限制') + '</td></tr>' +
            '<tr><td>最大RPM</td><td>' + (config.max_rpm || '无限制') + '</td></tr>' +
            '<tr><td>最大RPD</td><td>' + (config.max_rpd || '无限制') + '</td></tr>' +
            '<tr><td>请求间隔</td><td>' + (config.request_interval_ms || 0) + ' ms</td></tr>';
    } catch (error) {
        console.error('加载配置失败:', error);
    }
}

function changeTimeRange(range) {
    currentTimeRange = range;
    document.querySelectorAll('.time-btn').forEach(btn => btn.classList.remove('active'));
    event.target.classList.add('active');
    console.log('切换时间范围:', range);
}

function initCharts() {
    const tokenCtx = document.getElementById('tokenChart');
    if (tokenCtx) {
        tokenChart = new Chart(tokenCtx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [{
                    label: 'Token 使用量',
                    data: [],
                    borderColor: '#4CAF50',
                    backgroundColor: 'rgba(76, 175, 80, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        labels: { color: getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim() }
                    }
                },
                scales: {
                    x: {
                        ticks: { color: getComputedStyle(document.documentElement).getPropertyValue('--text-tertiary').trim() },
                        grid: { color: getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() }
                    },
                    y: {
                        ticks: { color: getComputedStyle(document.documentElement).getPropertyValue('--text-tertiary').trim() },
                        grid: { color: getComputedStyle(document.documentElement).getPropertyValue('--border-color').trim() }
                    }
                }
            }
        });
    }

    const successCtx = document.getElementById('successChart');
    if (successCtx) {
        successChart = new Chart(successCtx, {
            type: 'doughnut',
            data: {
                labels: ['成功', '失败'],
                datasets: [{
                    data: [0, 0],
                    backgroundColor: ['rgba(76, 175, 80, 0.8)', 'rgba(244, 67, 54, 0.8)'],
                    borderColor: ['#4CAF50', '#f44336'],
                    borderWidth: 2
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        position: 'bottom',
                        labels: { color: getComputedStyle(document.documentElement).getPropertyValue('--text-secondary').trim(), padding: 20 }
                    }
                }
            }
        });
    }
}

function updateSuccessChart(stats) {
    if (!successChart) return;
    successChart.data.datasets[0].data = [stats.completed || 0, stats.failed || 0];
    successChart.update();
}

document.addEventListener('DOMContentLoaded', () => {
    initCharts();
});

window.addEventListener('load', () => {
    setTimeout(async () => {
        await loadConfig();
        startDataUpdates();
    }, 1000);
});

window.addEventListener('beforeunload', () => {
    stopDataUpdates();
});

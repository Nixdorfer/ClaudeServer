async function loadVersionChanges() {
    try {
        const response = await fetch('/api/version-changes');
        const data = await response.json();

        const container = document.getElementById('changesList');

        if (!data || Object.keys(data).length === 0) {
            container.innerHTML = '<div class="no-versions">暂无版本更新记录</div>';
            return;
        }

        const versions = Object.keys(data).sort().reverse();

        versions.forEach(version => {
            const versionData = data[version];
            const card = document.createElement('div');
            card.className = 'version-card';

            const changesHTML = versionData.content.map(change =>
                `<li class="change-item">${change}</li>`
            ).join('');

            card.innerHTML = `
                <div class="version-header">
                    <div class="version-number">v${version}</div>
                    <div class="version-date">${versionData.time}</div>
                </div>
                <ul class="changes-list">
                    ${changesHTML}
                </ul>
            `;

            container.appendChild(card);
        });
    } catch (error) {
        console.error('加载版本更新失败:', error);
        document.getElementById('changesList').innerHTML =
            '<div class="no-versions">加载失败，请稍后重试</div>';
    }
}

loadVersionChanges();

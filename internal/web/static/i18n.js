// i18n.js — Chinese/English language pack
const I18N = {
  zh: {
    'tab.connections': '连接',
    'tab.crawl': '代理采集',
    'tab.pool': '代理池',
    'tab.logs': '运行日志',
    'stat.active': '连接',
    'stat.strategy': '策略',
    'conn.closeAll': '关闭所有',
    'conn.start': 'Start',
    'conn.type': 'Type',
    'conn.host': 'Host',
    'conn.target': 'Target',
    'conn.proxy': 'Proxy',
    'conn.source': 'Source',
    'conn.up': '▲ Up',
    'conn.down': '▼ Down',
    'conn.age': 'Age',
    'conn.empty': 'No active connections.',
    'crawl.fetch': '🔍 获取代理',
    'crawl.validate': '✅ 验证代理',
    'crawl.dynamic': '🔄 动态代理',
    'crawl.dynamicStop': '⏹ 停止',
    'crawl.dynamicRunning': '动态运行中',
    'crawl.dynamicStopped': '动态已停止',
    'crawl.intervalSec': '秒',
    'crawl.autoPoolOn': '自动入池(开)',
    'crawl.autoPoolOff': '🤖 自动入池',
    'crawl.autoPoolEnabled': '自动入池已开启',
    'crawl.autoPoolDisabled': '自动入池已关闭',
    'crawl.cycleInfo': '获取 {fetched} 可用 {valid} 入池 {added}',
    'crawl.addToPool': '📥 移入代理池',
    'crawl.addr': '地址',
    'crawl.type': '类型',
    'crawl.source': '来源',
    'crawl.latency': '延迟',
    'crawl.empty': '点击「获取代理」开始采集',
    'pool.strategy': '策略:',
    'pool.random': '随机',
    'pool.roundRobin': '轮询',
    'pool.hash': '哈希',
    'pool.name': '指定',
    'pool.save': '💾 保存到配置',
    'pool.colName': '名称',
    'pool.colType': '类型',
    'pool.colAddr': '地址',
    'pool.colHealth': '健康',
    'logs.all': '全部',
    'logs.info': 'Info',
    'logs.warn': 'Warn',
    'logs.error': 'Error',
    'logs.refresh': '🔄 刷新',
    'logs.clear': '🗑 清空',
  },
  en: {
    'tab.connections': 'Connections',
    'tab.crawl': 'Proxy Crawl',
    'tab.pool': 'Proxy Pool',
    'tab.logs': 'Logs',
    'stat.active': 'active',
    'stat.strategy': 'strategy',
    'conn.closeAll': 'Close All',
    'conn.start': 'Start',
    'conn.type': 'Type',
    'conn.host': 'Host',
    'conn.target': 'Target',
    'conn.proxy': 'Proxy',
    'conn.source': 'Source',
    'conn.up': '▲ Up',
    'conn.down': '▼ Down',
    'conn.age': 'Age',
    'conn.empty': 'No active connections.',
    'crawl.fetch': '🔍 Fetch Proxies',
    'crawl.validate': '✅ Validate',
    'crawl.dynamic': '🔄 Dynamic',
    'crawl.dynamicStop': '⏹ Stop',
    'crawl.dynamicRunning': 'Dynamic running',
    'crawl.dynamicStopped': 'Dynamic stopped',
    'crawl.intervalSec': 's',
    'crawl.autoPoolOn': 'Auto-Pool (ON)',
    'crawl.autoPoolOff': '🤖 Auto-Pool',
    'crawl.autoPoolEnabled': 'Auto-pool enabled',
    'crawl.autoPoolDisabled': 'Auto-pool disabled',
    'crawl.cycleInfo': 'fetched {fetched} valid {valid} pooled {added}',
    'crawl.addToPool': '📥 Add to Pool',
    'crawl.addr': 'Address',
    'crawl.type': 'Type',
    'crawl.source': 'Source',
    'crawl.latency': 'Latency',
    'crawl.empty': 'Click "Fetch Proxies" to start',
    'pool.strategy': 'Strategy:',
    'pool.random': 'Random',
    'pool.roundRobin': 'Round Robin',
    'pool.hash': 'Hash',
    'pool.name': 'Specific',
    'pool.save': '💾 Save Config',
    'pool.colName': 'Name',
    'pool.colType': 'Type',
    'pool.colAddr': 'Address',
    'pool.colHealth': 'Health',
    'logs.all': 'All',
    'logs.info': 'Info',
    'logs.warn': 'Warn',
    'logs.error': 'Error',
    'logs.refresh': '🔄 Refresh',
    'logs.clear': '🗑 Clear',
  }
};

let currentLang = 'zh';

function t(key) {
  return (I18N[currentLang] && I18N[currentLang][key]) || key;
}

function applyI18n() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.dataset.i18n;
    if (el.tagName === 'INPUT' && el.type === 'text') {
      el.placeholder = t(key);
    } else if (el.tagName === 'OPTION') {
      el.textContent = t(key);
    } else {
      el.textContent = t(key);
    }
  });
  // Re-render current view
  if (typeof refresh === 'function') refresh();
}

// Apply i18n on page load
applyI18n();

document.getElementById('lang-toggle').addEventListener('click', () => {
  currentLang = currentLang === 'zh' ? 'en' : 'zh';
  document.getElementById('lang-toggle').textContent = currentLang === 'zh' ? '中/EN' : 'EN/中';
  applyI18n();
});

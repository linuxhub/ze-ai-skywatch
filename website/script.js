(() => {
  'use strict';
  document.documentElement.classList.add('js');

  // Preset examples only: this page never reads system processes or calls AI.
  const examples = {
    chrome: {
      title: '这是你的网页浏览器。',
      summary: 'Google Chrome 负责网页渲染、标签页和扩展运行。多个浏览器进程共同工作是常见现象。',
      advice: '在浏览器任务管理器中检查标签页与扩展，并连续观察资源占用变化。',
      evidence: '基于进程摘要 · 单次采样不代表持续异常',
    },
    windowserver: {
      title: '它在绘制你的 macOS 桌面。',
      summary: 'WindowServer 是 macOS 的窗口与图形显示进程，负责把应用窗口、动画和屏幕内容呈现出来。',
      advice: '观察多显示器、窗口动画或高负载应用对占用的影响，避免直接停止系统关键进程。',
      evidence: '系统关键进程 · 先观察，再排查',
    },
    node: {
      title: '它可能正在运行 JavaScript 服务。',
      summary: 'node 通常是 Node.js 运行时，可能用于开发服务器或后台工具。仅凭名称还不能确认具体应用。',
      advice: '在应用中查看进程路径、所属用户与监听端口，结合这些线索判断它属于哪个项目。',
      evidence: '信息有限时明确边界 · 不凭名称判断风险',
    },
  };

  const processButtons = document.querySelectorAll('[data-process]');
  processButtons.forEach((button) => {
    button.addEventListener('click', () => {
      const example = examples[button.dataset.process];
      if (!example) return;
      processButtons.forEach((item) => {
        const selected = item === button;
        item.classList.toggle('active', selected);
        item.setAttribute('aria-pressed', String(selected));
      });
      document.getElementById('demo-title').textContent = example.title;
      document.getElementById('demo-summary').textContent = example.summary;
      document.getElementById('demo-advice').textContent = example.advice;
      document.getElementById('demo-evidence').textContent = example.evidence;
    });
  });

  const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
  function selectTab(tab, focus = false) {
    tabs.forEach((item) => {
      const selected = item === tab;
      item.classList.toggle('active', selected);
      item.setAttribute('aria-selected', String(selected));
      item.tabIndex = selected ? 0 : -1;
      document.getElementById(item.getAttribute('aria-controls')).hidden = !selected;
    });
    if (focus) tab.focus();
  }
  tabs.forEach((tab, index) => {
    tab.addEventListener('click', () => selectTab(tab));
    tab.addEventListener('keydown', (event) => {
      let next;
      if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
      if (event.key === 'Home') next = 0;
      if (event.key === 'End') next = tabs.length - 1;
      if (next !== undefined) {
        event.preventDefault();
        selectTab(tabs[next], true);
      }
    });
  });

  const menuButton = document.querySelector('.menu-toggle');
  const navigation = document.getElementById('main-nav');
  function closeMenu() {
    menuButton.setAttribute('aria-expanded', 'false');
    menuButton.setAttribute('aria-label', '展开导航');
    navigation.classList.remove('open');
  }
  menuButton.addEventListener('click', () => {
    const expanded = menuButton.getAttribute('aria-expanded') !== 'true';
    menuButton.setAttribute('aria-expanded', String(expanded));
    menuButton.setAttribute('aria-label', expanded ? '收起导航' : '展开导航');
    navigation.classList.toggle('open', expanded);
  });
  navigation.querySelectorAll('a').forEach((link) => link.addEventListener('click', closeMenu));
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && menuButton.getAttribute('aria-expanded') === 'true') {
      closeMenu();
      menuButton.focus();
    }
  });
  document.addEventListener('click', (event) => {
    if (!event.target.closest('.header-inner')) closeMenu();
  });
})();

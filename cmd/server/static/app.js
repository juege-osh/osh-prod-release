(() => {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const state = {
    health: null, current: null, currentBlue: null, pollTimer: null, blueDeployPollTimer: null,
    busy: false, page: 'deploy', activeDeploy: null, activePollTimer: null, traffic: null,
    trafficLoading: false, blueActive: null, bluePollTimer: null,
    componentSyncActive: null, componentSyncPollTimer: null,
    warRoom: { items: [], view: 'table', activeComponent: 'mysql', specs: [], executions: [], reports: [], conflicts: [] },
    componentOpsTab: 'mysql',
    componentOpsQueue: [],
    adminUsers: [],
    user: null, token: sessionStorage.getItem('osh_token') || '', authMode: 'login',
  };

  const WAR_COMPONENTS = {
    mysql: {
      label: 'MySQL',
      summary: 'MySQL · 数据库变更',
      type: 'migration',
      action: 'apply',
      title: '例：新增订单扩展字段',
      ref: 'SQL 文件路径或迁移说明，例如 /opt/osh-prod-release/migrations/20260708_order.sql',
      node: '例：osh-g-mysql 或 all',
      impact: '说明会影响哪些表、接口、后台页面',
      data: '例：alter order_ext 新增 channel 字段；预计新增 0 行，修改 1 张表',
      test: '执行接口回归 + 查询表结构 + 对比上线前后行数/字段',
      rollback: '自动快照恢复；必要时执行回滚 SQL',
      guide: '填写 SQL 文件或迁移说明。平台会先在绿库执行前做快照，失败自动恢复。',
    },
    redis: {
      label: 'Redis',
      summary: 'Redis · 缓存 / key 变更',
      type: 'component',
      action: 'apply',
      title: '例：新增登录态缓存 key',
      ref: 'Redis 命令文件路径，或 key 前缀说明',
      node: '例：osh-g-redis 或 all',
      impact: '说明新增/修改哪些缓存 key，是否影响登录、支付、任务',
      data: '例：新增 session:* key，TTL 2 小时，预计数量 1 万以内',
      test: 'PING + key 采样 + 业务接口读取缓存验证',
      rollback: '删除新增 key 或恢复上线前 RDB/快照',
      guide: '适合缓存 key、TTL、预热数据变更。不要直接写蓝 Redis，先在绿环境验证。',
    },
    nacos: {
      label: 'Nacos',
      summary: 'Nacos · 配置变更',
      type: 'config',
      action: 'apply',
      title: '例：调整支付回调配置',
      ref: 'dataId / group / namespace，或配置脚本路径',
      node: '例：osh-g-nacos 或 namespace',
      impact: '说明哪些服务会读取该配置，是否需要重启',
      data: '例：修改 payment.callback.timeout，从 3s 到 5s',
      test: '检查配置版本 + 服务读取配置 + 接口回归',
      rollback: '恢复 Nacos 配置快照或上一版本',
      guide: '填写 dataId、group、namespace 和影响服务。上线后重点验证服务是否读到绿环境配置。',
    },
    es: {
      label: 'ES',
      summary: 'ES · 索引 / mapping',
      type: 'component',
      action: 'apply',
      title: '例：新增课程搜索字段',
      ref: '索引名 / mapping 文件 / 脚本路径',
      node: '例：course_index 或 all',
      impact: '说明影响哪些搜索接口、索引和字段',
      data: '例：course_index 新增 teacher_name keyword 字段，需回填 20 万文档',
      test: 'cluster health + mapping 检查 + 搜索接口抽样',
      rollback: '恢复索引快照或回滚 mapping/alias',
      guide: '适合索引、mapping、alias、回填任务。先在绿 ES 看 health 和文档量变化。',
    },
    kafka: {
      label: 'Kafka',
      summary: 'Kafka · topic / 消费链路',
      type: 'component',
      action: 'create-topic',
      title: '例：新增支付事件 topic',
      ref: 'topic 名称，例如 osh.payment.event',
      node: '分区数，例如 3',
      impact: '说明生产者、消费者、消息格式和失败影响',
      data: '例：新增 topic osh.payment.event，partition=3，日增 10 万消息',
      test: 'topic list + 生产/消费 smoke + 消费组 lag 检查',
      rollback: '若为新建 topic 且无业务使用，可删除 topic；否则停止生产者',
      guide: 'Kafka 页签把“上线引用”当 topic 名，“目标节点”当分区数。适合小白直接填。',
    },
    mongodb: {
      label: 'MongoDB',
      summary: 'MongoDB · collection / index',
      type: 'component',
      action: 'apply',
      title: '例：新增用户画像集合索引',
      ref: 'collection / index 脚本路径',
      node: '例：osh-g-mongodb 或 collection',
      impact: '说明影响哪些集合、查询和接口',
      data: '例：user_profile 新增 unionId_1 索引，预计 50 万文档',
      test: 'mongo ping + explain + 接口查询抽样',
      rollback: 'mongodump 快照恢复或删除新增索引',
      guide: '适合新增 collection、索引、字段回填。未部署 MongoDB 时会作为扩展组件记录。',
    },
    hbase: {
      label: 'HBase',
      summary: 'HBase · 表 / 列族',
      type: 'component',
      action: 'apply',
      title: '例：新增行为日志列族',
      ref: '表名 / shell 脚本路径',
      node: '例：osh-g-hbase 或 table',
      impact: '说明影响哪些表、列族、离线任务和查询',
      data: '例：behavior_log 新增 cf_ext 列族，预计日增 200 万行',
      test: 'hbase shell status + table exists + row count 抽样',
      rollback: 'HBase snapshot 恢复或删除新增列族/表',
      guide: '适合表、列族、批量导入类变更。先写清楚数据量和回滚窗口。',
    },
    application: {
      label: '代码',
      summary: 'Java / Frontend · 应用功能',
      type: 'code',
      action: 'deploy',
      title: '例：发布订单列表筛选功能',
      ref: 'git branch / commit / 构建产物说明',
      node: '例：osh-g-backend、osh-g-frontend 或 all',
      impact: '说明影响哪些页面、接口、权限和用户',
      data: '如无数据变更写“无”；如有接口新增字段请写清楚',
      test: '接口回归 + 页面人工验收 + 日志错误检查',
      rollback: '回滚到上一个 commit 或部署快照',
      guide: '适合 Java 后端、前端页面和普通功能发布。代码上线也必须填评审和测试计划。',
    },
  };

  function isAdmin() {
    return !!state.user?.is_admin;
  }

  function isBoss() {
    return !!state.user?.is_boss;
  }

  function authHeaders() {
    const h = { 'Content-Type': 'application/json' };
    if (state.token) h.Authorization = `Bearer ${state.token}`;
    return h;
  }

  function showApp(loggedIn) {
    const login = $('loginScreen');
    const app = $('appLayout');
    if (!login || !app) return;
    if (loggedIn) {
      login.hidden = true;
      login.setAttribute('hidden', '');
      app.hidden = false;
      app.removeAttribute('hidden');
    } else {
      login.hidden = false;
      login.removeAttribute('hidden');
      app.hidden = true;
      app.setAttribute('hidden', '');
    }
  }

  function renderUserBadge() {
    const badge = $('userBadge');
    const logout = $('btnLogout');
    if (!badge) return;
    if (!state.user) {
      badge.hidden = true;
      if (logout) logout.hidden = true;
      return;
    }
    badge.hidden = false;
    badge.textContent = isAdmin() ? `${state.user.display_name || state.user.username} · 管理员` : (state.user.display_name || state.user.username);
    badge.className = `badge user-badge${isAdmin() ? ' admin' : ''}`;
    if (logout) logout.hidden = false;
    document.querySelectorAll('.nav-item.admin-only').forEach((el) => {
      el.hidden = !isAdmin();
    });
    const author = $('author');
    const blueAuthor = $('blueAuthor');
    if (author) {
      author.value = state.user.username;
      author.readOnly = true;
    }
    if (blueAuthor) {
      blueAuthor.value = state.user.username;
      blueAuthor.readOnly = true;
    }
  }

  async function login() {
    const username = $('loginUser').value.trim();
    const password = $('loginPass').value;
    const errEl = $('loginError');
    errEl.hidden = true;
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || '登录失败');
      state.token = data.token;
      state.user = data.user;
      sessionStorage.setItem('osh_token', state.token);
      showApp(true);
      renderUserBadge();
      await bootApp();
    } catch (err) {
      errEl.hidden = false;
      errEl.textContent = err.message || String(err);
    }
  }

  async function register() {
    const username = $('loginUser').value.trim();
    const password = $('loginPass').value;
    const password2 = $('registerPass2')?.value || '';
    const displayName = $('registerDisplayName')?.value.trim() || username;
    const errEl = $('loginError');
    errEl.hidden = true;
    if (password !== password2) {
      errEl.hidden = false;
      errEl.textContent = '两次输入的密码不一致';
      return;
    }
    try {
      const res = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password, display_name: displayName }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || '注册失败');
      state.token = data.token;
      state.user = data.user;
      sessionStorage.setItem('osh_token', state.token);
      showApp(true);
      renderUserBadge();
      await bootApp();
    } catch (err) {
      errEl.hidden = false;
      errEl.textContent = err.message || String(err);
    }
  }

  async function submitAuth() {
    if (state.authMode === 'register') return register();
    return login();
  }

  function renderAuthMode() {
    const registering = state.authMode === 'register';
    document.querySelectorAll('.register-only').forEach((el) => {
      el.hidden = !registering;
      if (registering) el.removeAttribute('hidden');
      else el.setAttribute('hidden', '');
    });
    const pass = $('loginPass');
    if (pass) {
      pass.placeholder = registering ? '至少 8 位密码' : '请输入密码';
      pass.autocomplete = registering ? 'new-password' : 'current-password';
    }
    const btn = $('btnLogin');
    if (btn) btn.textContent = registering ? '创建账号并进入 →' : '登录 →';
    const sw = $('btnAuthSwitch');
    if (sw) sw.textContent = registering ? '返回登录' : '创建普通账号';
    const hint = $('loginHint');
    if (hint) hint.textContent = registering
      ? '新账号默认为普通成员；发布仍需两位评审人测试通过和觉哥终审。'
      : '管理员 juege 可直通发布；其他用户需双评审 + 终审。';
    const errEl = $('loginError');
    if (errEl) errEl.hidden = true;
  }

  function toggleAuthMode() {
    state.authMode = state.authMode === 'login' ? 'register' : 'login';
    renderAuthMode();
  }

  async function logout() {
    try {
      await fetch('/api/auth/logout', { method: 'POST', headers: authHeaders() });
    } catch { /* ignore */ }
    state.token = '';
    state.user = null;
    sessionStorage.removeItem('osh_token');
    showApp(false);
  }

  async function restoreSession() {
    if (!state.token) {
      showApp(false);
      return false;
    }
    try {
      const res = await fetch('/api/auth/me', { headers: authHeaders() });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || 'session expired');
      state.user = data;
      showApp(true);
      renderUserBadge();
      return true;
    } catch {
      state.token = '';
      sessionStorage.removeItem('osh_token');
      showApp(false);
      return false;
    }
  }

  async function loadDeploySnapshotsFor(target, listId, btnId) {
    const el = $(listId);
    const btn = $(btnId);
    if (!el) return;
    if (!isAdmin()) {
      el.innerHTML = '<div class="empty">仅管理员可查看回滚</div>';
      if (btn) btn.hidden = true;
      return;
    }
    try {
      const list = await api(`/api/deploy/snapshots?target=${target}`);
      if (!list.length) {
        el.innerHTML = '<div class="empty">暂无部署快照</div>';
        if (btn) btn.hidden = true;
        return;
      }
      el.innerHTML = list.slice(0, 5).map((s, i) => `
        <div class="rollback-item">
          <strong>${i === 0 ? '当前' : '历史'} · ${escapeHtml(s.title)}</strong>
          <span class="meta">${new Date(s.created_at).toLocaleString()} · ${escapeHtml(s.backend_git_ref)} / ${escapeHtml(s.frontend_git_ref)}</span>
        </div>`).join('');
      if (btn) btn.hidden = list.length < 2;
    } catch {
      el.innerHTML = '<div class="empty">加载失败</div>';
      if (btn) btn.hidden = true;
    }
  }

  async function loadDeploySnapshots() {
    await loadDeploySnapshotsFor('green', 'rollbackList', 'btnRollbackGreen');
    await loadDeploySnapshotsFor('blue', 'blueRollbackList', 'btnRollbackBlue');
  }

  async function rollbackDeploy(target) {
    if (!isAdmin()) {
      toast('仅管理员可执行版本回滚', 'error');
      return;
    }
    const label = target === 'blue' ? '蓝环境' : '绿环境';
    if (!confirm(`确认将${label}回滚到上一成功部署版本？\n\n将通过 GHA 重新部署上一版本的 git ref。`)) return;
    state.busy = true;
    $('loading').classList.add('show');
    try {
      const snap = await api('/api/deploy/rollback', {
        method: 'POST',
        body: JSON.stringify({ target, to_previous: true, actor: state.user.username, reason: 'manual rollback' }),
      });
      toast(`回滚完成：${snap.title || snap.id}`);
      await loadDeploySnapshots();
    } catch (err) {
      toast(err.message || String(err), 'error');
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
    }
  }

  const PAGE_GROUPS = {
    deploy: 'green',
    'war-room': 'green',
    components: 'green',
    'auto-test': 'green',
    sql: 'green',
    'sync-green': 'green',
    traffic: null,
    users: null,
    'deploy-blue': 'blue',
    'sql-blue': 'blue',
    'sync-blue': 'blue',
  };

  const PAGES = {
    deploy: {
      title: '部署绿环境',
      subtitle: '4 步向导 · GitHub Actions · 不影响蓝环境生产',
    },
    components: {
      title: '绿环境组件上线',
      subtitle: 'MySQL / Redis / Nacos / ES / Kafka · 快照 · 增量 · 回滚',
    },
    'auto-test': {
      title: '自动化测试',
      subtitle: '功能探测 · 数据 diff · AI 判定 · 不执行上线',
    },
    sql: {
      title: '绿环境组件上线',
      subtitle: 'MySQL / Redis / Nacos / ES / Kafka · 快照 · 增量 · 回滚',
    },
    'war-room': {
      title: 'Change 作战台',
      subtitle: '组件级增量上线 · 先绿测试 · 切流后同步蓝',
    },
    'sync-green': {
      title: '蓝到绿环境基线同步',
      subtitle: '运维定时任务 · 不是 change 上线能力',
    },
    traffic: {
      title: '生产切流',
      subtitle: '蓝 ↔ 绿 · 149 :80 入口切换 · 与部署独立',
    },
    users: {
      title: '用户管理',
      subtitle: '管理员创建用户 · 调整角色 · 重置密码',
    },
    'deploy-blue': {
      title: '部署蓝环境',
      subtitle: '4 步向导 · GHA slot=blue · 验收 :58080',
    },
    'sql-blue': {
      title: '更新蓝环境数据库',
      subtitle: '自定义 SQL · 仅 osh-mysql · 增量更新（非全量同步）',
    },
    'sync-blue': {
      title: '全量同步蓝环境数据库',
      subtitle: '绿 → 蓝 · MySQL/Nacos/ES/Redis · 覆盖蓝库中间件',
    },
  };

  const STEPS = {
    1: {
      badge: '第 1 步',
      title: '填写发布信息',
      desc: '给这次部署起个名字，填上你的名字，然后点下面绿色按钮。',
      btn: '下一步：创建发布单',
      showForm: true,
    },
    2: {
      badge: '第 2 步',
      title: '提交评审',
      desc: '发布单已创建。点按钮把它提交给评审流程。',
      btn: '下一步：提交评审',
      showForm: false,
    },
    3: {
      badge: '第 3 步',
      title: '完成审批',
      desc: '模拟两位评审通过 + 负责人终审（内网测试流程，点一次即可）。',
      btn: '下一步：完成审批',
      showForm: false,
    },
    4: {
      badge: '第 4 步',
      title: '部署到绿环境',
      desc: '将触发 GitHub Actions，把前后端代码部署到 149 的 :28080 绿环境。',
      btn: '开始部署到绿环境',
      showForm: false,
    },
  };

  const BLUE_STEPS = {
    1: {
      badge: '第 1 步',
      title: '填写发布信息',
      desc: '给这次蓝环境部署起个名字，填上你的名字，然后点下面蓝色按钮。',
      btn: '下一步：创建发布单',
      showForm: true,
    },
    2: {
      badge: '第 2 步',
      title: '提交评审',
      desc: '发布单已创建。点按钮把它提交给评审流程。',
      btn: '下一步：提交评审',
      showForm: false,
    },
    3: {
      badge: '第 3 步',
      title: '完成审批',
      desc: '模拟两位评审通过 + 负责人终审（内网测试流程，点一次即可）。',
      btn: '下一步：完成审批',
      showForm: false,
    },
    4: {
      badge: '第 4 步',
      title: '部署到蓝环境',
      desc: '将触发 GitHub Actions slot=blue，部署到 /opt/osh/app，验收 :58080。前提：生产流量须在绿环境。',
      btn: '开始部署到蓝环境',
      showForm: false,
    },
  };

  function releaseTarget(rel) {
    return rel?.deploy_target || 'green';
  }

  function filterByTarget(list, target) {
    return (list || []).filter((r) => releaseTarget(r) === target);
  }

  function navigate(page) {
    if (page === 'sql') page = 'components';
    if (!PAGES[page]) return;
    state.page = page;
    localStorage.setItem('osh_page', page);
    if (location.hash !== `#${page}`) {
      history.replaceState(null, '', `#${page}`);
    }

    document.querySelectorAll('.nav-item').forEach((el) => {
      const navPage = el.dataset.page === 'sql' ? 'components' : el.dataset.page;
      el.classList.toggle('active', navPage === page);
    });
    expandNavGroupForPage(page);
    document.querySelectorAll('.page').forEach((el) => {
      const active = el.dataset.page === page;
      el.classList.toggle('active', active);
      el.hidden = !active;
    });

    $('pageTitle').textContent = PAGES[page].title;
    $('pageSubtitle').textContent = PAGES[page].subtitle;

    if (page === 'traffic') {
      loadTrafficPage();
    }
    if (page === 'sync-green') {
      loadTrafficForGreenSyncPage();
      loadComponentSyncActive();
    }
    if (page === 'war-room') {
      loadWarRoom();
    }
    if (page === 'components') {
      renderComponentOpsTab();
      loadComponentOpsHistory();
    }
    if (page === 'auto-test') {
      loadAutoTestReport();
    }
    if (page === 'users') {
      loadAdminUsers();
    }
    if (page === 'deploy-blue') {
      loadTrafficForBluePages();
      renderBlueUI();
      renderBlueDeployUI();
      loadBlueList();
    }
    if (page === 'sync-blue' || page === 'sql-blue') {
      loadTrafficForBluePages();
      if (page === 'sync-blue') loadBlueActive();
      if (page === 'sql-blue') renderBlueSqlUI();
    }
  }

  async function loadTrafficForBluePages() {
    await loadTraffic();
    if ((state.traffic?.active || 'unknown') === 'unknown') {
      setTimeout(loadTraffic, 2000);
    }
  }

  async function loadTrafficForGreenSyncPage() {
    await loadTraffic();
    if ((state.traffic?.active || 'unknown') === 'unknown') {
      setTimeout(loadTraffic, 2000);
    }
    renderGreenSyncUI();
  }

  function setNavGroupOpen(group, open) {
    const el = document.querySelector(`.nav-group[data-group="${group}"]`);
    if (!el) return;
    const head = el.querySelector('.nav-group-head');
    const body = el.querySelector('.nav-group-body');
    if (!head || !body) return;
    head.setAttribute('aria-expanded', open ? 'true' : 'false');
    el.classList.toggle('open', open);
    if (open) {
      body.removeAttribute('hidden');
    } else {
      body.setAttribute('hidden', '');
    }
    localStorage.setItem(`osh_nav_${group}_open`, open ? '1' : '0');
  }

  function toggleNavGroup(group) {
    const el = document.querySelector(`.nav-group[data-group="${group}"]`);
    const willOpen = !el?.classList.contains('open');
    if (willOpen) {
      const other = group === 'green' ? 'blue' : 'green';
      setNavGroupOpen(other, false);
    }
    setNavGroupOpen(group, willOpen);
  }

  function expandNavGroupForPage(page) {
    const group = PAGE_GROUPS[page];
    if (group) {
      const other = group === 'green' ? 'blue' : 'green';
      setNavGroupOpen(other, false);
      setNavGroupOpen(group, true);
      return;
    }
    // 生产切流：收起蓝绿分组，减少干扰
    if (page === 'traffic') {
      setNavGroupOpen('green', false);
      setNavGroupOpen('blue', false);
    }
  }

  function initNavGroups() {
    const page = state.page || localStorage.getItem('osh_page') || 'deploy';
    const activeGroup = PAGE_GROUPS[page];

    ['green', 'blue'].forEach((group) => {
      if (page === 'traffic') {
        setNavGroupOpen(group, false);
        return;
      }
      if (activeGroup) {
        setNavGroupOpen(group, group === activeGroup);
        return;
      }
      const stored = localStorage.getItem(`osh_nav_${group}_open`);
      setNavGroupOpen(group, stored === null ? group === 'green' : stored === '1');
    });

    document.querySelectorAll('[data-toggle-group]').forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        toggleNavGroup(btn.dataset.toggleGroup);
      });
    });
  }

  function initSidebar() {
    const layout = $('appLayout');
    if (!layout) return;
    if (localStorage.getItem('osh_sidebar_collapsed') === '1') {
      layout.classList.add('collapsed');
    }

    $('sidebarToggle')?.addEventListener('click', () => {
      layout.classList.toggle('collapsed');
      localStorage.setItem('osh_sidebar_collapsed', layout.classList.contains('collapsed') ? '1' : '0');
    });

    $('mobileMenuBtn')?.addEventListener('click', () => {
      layout.classList.toggle('mobile-open');
      const backdrop = $('sidebarBackdrop');
      if (backdrop) backdrop.hidden = !layout.classList.contains('mobile-open');
    });

    $('sidebarBackdrop')?.addEventListener('click', () => {
      layout.classList.remove('mobile-open');
      const backdrop = $('sidebarBackdrop');
      if (backdrop) backdrop.hidden = true;
    });

    document.querySelectorAll('.nav-item').forEach((btn) => {
      btn.addEventListener('click', () => {
        navigate(btn.dataset.page);
        layout.classList.remove('mobile-open');
        $('sidebarBackdrop').hidden = true;
      });
    });
  }

  function initPageFromHash() {
    let hash = (location.hash || '').replace('#', '').replace(/^\//, '');
    if (hash === 'sql') hash = 'components';
    const page = PAGES[hash] ? hash : (localStorage.getItem('osh_page') === 'sql' ? 'components' : (localStorage.getItem('osh_page') || 'deploy'));
    state.page = page;
    initNavGroups();
    navigate(page);
  }

  function isOtherDeployBusy(rel) {
    const current = rel || state.current;
    return state.activeDeploy?.busy && state.activeDeploy.id !== current?.id;
  }

  function isAnyDeployBusy() {
    return !!state.activeDeploy?.busy;
  }

  function isProductionGreen() {
    return (state.traffic?.active || '') === 'green';
  }

  function isProductionBlue() {
    return (state.traffic?.active || '') === 'blue';
  }

  function isBlueBusy() {
    return !!state.blueActive?.busy;
  }

  function isComponentSyncBusy() {
    return !!state.componentSyncActive?.busy;
  }

  function canOperateBlue() {
    return isProductionGreen() && !isAnyDeployBusy() && !isBlueBusy() && !state.busy;
  }

  function canOperateBlueSql() {
    return canOperateBlue();
  }

  function canOperateGreenSync() {
    return isProductionBlue() && !isAnyDeployBusy() && !isComponentSyncBusy() && !state.busy;
  }

  function productionTrafficBanner(context) {
    if (state.trafficLoading) {
      return { show: true, kind: 'loading', icon: '⏳', text: '正在检测当前生产流量…' };
    }
    const active = state.traffic?.active || 'unknown';
    if (active === 'green') {
      let text = '当前生产为绿环境，可更新部署蓝环境前后端';
      if (context === 'sync') text = '当前生产为绿环境，可执行绿 → 蓝全量数据同步';
      if (context === 'sql') text = '当前生产为绿环境，可对蓝待命库执行增量 SQL';
      return { show: true, kind: 'ok', icon: '✓', text };
    }
    if (active === 'blue') {
      return {
        show: true,
        kind: 'warn',
        icon: '⚠️',
        text: '当前生产为蓝环境，请先切流到绿环境后再操作蓝环境。',
      };
    }
    return {
      show: true,
      kind: 'warn',
      icon: '⚠️',
      text: '暂时无法确认生产流量，请稍后重试或到「生产切流」页查看。',
    };
  }

  function productionGuardMessage() {
    const active = state.traffic?.active || 'unknown';
    if (active === 'green') return '';
    if (active === 'blue') {
      return '当前生产为蓝环境，请先切流到绿环境后再操作蓝环境。';
    }
    return '暂时无法确认生产流量，请稍后重试。';
  }

  function greenSyncGuardMessage() {
    const active = state.traffic?.active || 'unknown';
    if (active === 'blue') return '';
    if (active === 'green') return '当前生产为绿环境，不能再用蓝环境覆盖绿环境。';
    return '暂时无法确认生产流量，请稍后重试。';
  }

  function renderGreenSyncGuard() {
    const banner = $('greenSyncGuardBanner');
    const text = $('greenSyncGuardText');
    const iconEl = banner?.querySelector('.lock-icon');
    if (!banner || !text) return;
    const active = state.traffic?.active || 'unknown';
    const loading = state.trafficLoading;
    banner.hidden = false;
    banner.classList.remove('warn', 'ok', 'self', 'loading');
    if (loading) {
      banner.classList.add('self');
      if (iconEl) iconEl.textContent = '⏳';
      text.textContent = '正在检测当前生产流量…';
      return;
    }
    if (active === 'blue') {
      banner.classList.add('ok');
      if (iconEl) iconEl.textContent = '✓';
      text.textContent = '当前生产为蓝环境，可以把蓝环境基线同步到绿环境用于测试。';
      return;
    }
    banner.classList.add('warn');
    if (iconEl) iconEl.textContent = '⚠️';
    text.textContent = greenSyncGuardMessage();
  }

  function renderGreenSyncLock() {
    const banner = $('greenSyncLockBanner');
    const text = $('greenSyncLockText');
    if (!banner || !text) return;
    if (!isComponentSyncBusy()) {
      banner.hidden = true;
      return;
    }
    banner.hidden = false;
    text.textContent = state.componentSyncActive?.job?.message || '蓝→绿环境基线同步进行中…';
  }

  function renderGreenSyncResult() {
    const resultEl = $('greenSyncResult');
    const job = state.componentSyncActive?.job;
    if (!resultEl || !job) return;
    if (job.status === 'running') {
      resultEl.hidden = true;
      return;
    }
    resultEl.hidden = false;
    resultEl.className = `sql-result ${job.status === 'success' ? 'ok' : 'err'}`;
    const icon = job.status === 'success' ? '✓' : '✗';
    resultEl.textContent = `${icon} ${job.message || job.status}\n${job.output || ''}`;
  }

  function greenSyncHintText() {
    if (!isProductionBlue()) return greenSyncGuardMessage();
    if (isAnyDeployBusy()) return '有发布单正在部署中，请等待完成。';
    if (isComponentSyncBusy()) return state.componentSyncActive?.job?.message || '蓝→绿环境基线同步进行中…';
    return '执行 run-incremental-blue-to-green-all-components.sh，只写绿环境；真正上线请走 Change 作战台。';
  }

  function renderGreenSyncUI() {
    renderGreenSyncGuard();
    renderGreenSyncLock();
    const btn = $('btnSyncGreenAll');
    const hint = $('greenSyncHint');
    const tip = $('greenSyncTip');
    if (btn) {
      btn.disabled = !canOperateGreenSync();
      btn.title = !isProductionBlue() ? '生产须在蓝环境' : isAnyDeployBusy() ? '部署进行中' : isComponentSyncBusy() ? '同步进行中' : '';
    }
    if (hint) hint.textContent = greenSyncHintText();
    if (tip) {
      tip.textContent = isProductionBlue()
        ? '方向：蓝（生产）→ 绿（测试基线）· 运维基线同步'
        : '方向保护：仅允许生产在蓝环境时执行，避免覆盖绿生产。';
    }
    renderGreenSyncResult();
  }

  async function loadComponentSyncActive() {
    try {
      state.componentSyncActive = await api('/api/component-sync/active');
    } catch {
      state.componentSyncActive = null;
    }
    renderGreenSyncUI();
    scheduleComponentSyncPoll();
  }

  function scheduleComponentSyncPoll() {
    if (state.componentSyncPollTimer) clearInterval(state.componentSyncPollTimer);
    state.componentSyncPollTimer = null;
    if (!isComponentSyncBusy()) return;
    state.componentSyncPollTimer = setInterval(async () => {
      await loadComponentSyncActive();
      if (!isComponentSyncBusy()) {
        clearInterval(state.componentSyncPollTimer);
        state.componentSyncPollTimer = null;
        renderGreenSyncUI();
      }
    }, 5000);
  }

  async function loadBlueActive() {
    try {
      state.blueActive = await api('/api/blue/active');
    } catch {
      state.blueActive = null;
    }
    renderBlueUI();
    renderBlueSqlUI();
    scheduleBluePoll();
  }

  function renderBlueSqlGuard() {
    const banner = $('blueSqlGuardBanner');
    const text = $('blueSqlGuardText');
    const iconEl = banner?.querySelector('.lock-icon');
    if (!banner || !text) return;
    const b = productionTrafficBanner('sql');
    banner.hidden = !b.show;
    banner.classList.remove('warn', 'ok', 'self', 'loading');
    if (b.kind === 'ok') banner.classList.add('ok');
    else if (b.kind === 'loading') banner.classList.add('self');
    else banner.classList.add('warn');
    if (iconEl) iconEl.textContent = b.icon;
    text.textContent = b.text;
  }

  function renderBlueSqlLock() {
    const banner = $('blueSqlLockBanner');
    const text = $('blueSqlLockText');
    if (!banner || !text) return;
    if (!isBlueBusy()) {
      banner.hidden = true;
      return;
    }
    const job = state.blueActive?.job;
    const kindLabel = job?.kind === 'deploy' ? '蓝环境部署' : '绿→蓝数据同步';
    banner.hidden = false;
    text.textContent = `${kindLabel}进行中，请等待完成后再执行 SQL`;
  }

  function renderBlueSqlUI() {
    renderBlueSqlGuard();
    renderBlueSqlLock();
    const hint = $('blueSqlHint');
    const btn = $('btnExecBlueSql');
    if (hint) {
      hint.textContent = isProductionGreen()
        ? '当前生产为绿环境，蓝库为待命库，可执行增量 SQL。'
        : productionGuardMessage() || '正在检测生产流量…';
    }
    if (btn) btn.disabled = !canOperateBlueSql();
  }

  function renderBlueGuard(prefix) {
    const banner = $(`${prefix}GuardBanner`);
    const text = $(`${prefix}GuardText`);
    const iconEl = banner?.querySelector('.lock-icon');
    if (!banner || !text) return;
    const ctx = prefix === 'blueSync' ? 'sync' : prefix === 'blueSql' ? 'sql' : 'deploy';
    const b = productionTrafficBanner(ctx);
    banner.hidden = !b.show;
    banner.classList.remove('warn', 'ok', 'self', 'loading');
    if (b.kind === 'ok') banner.classList.add('ok');
    else if (b.kind === 'loading') banner.classList.add('self');
    else banner.classList.add('warn');
    if (iconEl) iconEl.textContent = b.icon;
    text.textContent = b.text;
  }

  function renderBlueLock(prefix) {
    const banner = $(`${prefix}LockBanner`);
    const text = $(`${prefix}LockText`);
    if (!banner || !text) return;
    if (!isBlueBusy()) {
      banner.hidden = true;
      return;
    }
    const job = state.blueActive.job;
    const kindLabel = job.kind === 'deploy' ? '蓝环境部署' : '绿→蓝数据同步';
    banner.hidden = false;
    text.textContent = `${kindLabel}进行中：${job.message || '…'}`;
  }

  function renderBlueJobResult(resultId, kind) {
    const resultEl = $(resultId);
    const job = state.blueActive?.job;
    if (!resultEl || !job || job.kind !== kind) return;
    if (job.status === 'running') {
      resultEl.hidden = true;
      return;
    }
    if (job.status === 'success' || job.status === 'failed') {
      resultEl.hidden = false;
      resultEl.className = `sql-result ${job.status === 'success' ? 'ok' : 'err'}`;
      const icon = job.status === 'success' ? '✓' : '✗';
      resultEl.textContent = `${icon} ${job.message || job.status}\n${job.output || ''}`;
    }
  }

  function blueHintText() {
    if (!isProductionGreen()) return '生产流量不在绿环境，暂不可操作。';
    if (isAnyDeployBusy()) return '有发布单正在部署中，请等待完成。';
    if (isBlueBusy()) {
      const job = state.blueActive.job;
      return job.message || '数据同步进行中…';
    }
    return '执行 osh-prod-standby-sync.sh --green-to-blue，耗时约 5–20 分钟。';
  }

  function renderBlueSyncUI() {
    renderBlueGuard('blueSync');
    renderBlueLock('blueSync');

    const syncBtn = $('btnSyncBlue');
    const syncHint = $('blueSyncHint');
    const syncOk = isProductionGreen() && !isAnyDeployBusy() && !isBlueBusy() && !state.busy;

    if (syncBtn) {
      syncBtn.disabled = !syncOk;
      syncBtn.title = !isProductionGreen() ? '生产须在绿环境' : isAnyDeployBusy() ? '部署进行中' : isBlueBusy() ? '同步进行中' : '';
    }
    if (syncHint) syncHint.textContent = blueHintText();
    renderBlueJobResult('blueSyncResult', 'sync');
  }

  function renderBlueUI() {
    renderBlueGuard('blueDeploy');
    renderBlueDeployLock();
    renderBlueSyncUI();
    renderBlueSqlUI();
  }

  function renderBlueDeployLock() {
    const banner = $('blueDeployLockBanner');
    const text = $('blueDeployLockText');
    if (!banner || !text) return;
    if (!isAnyDeployBusy()) {
      banner.hidden = true;
      return;
    }
    const d = state.activeDeploy;
    const isOther = d?.id !== state.currentBlue?.id;
    banner.hidden = false;
    text.textContent = isOther
      ? `发布单「${d.title}」正在部署中，请等待完成后再发起新的部署`
      : `当前发布单正在部署中，请等待完成…`;
  }

  function renderBlueStepper(step) {
    document.querySelectorAll('#blueStepper .stepper-item').forEach((el) => {
      const n = Number(el.dataset.step);
      el.classList.remove('active', 'done');
      const adminSkip = isAdmin() && step === 4 && (n === 2 || n === 3);
      if (step === 0 || (step > 0 && n < step) || adminSkip) el.classList.add('done');
      if (n === step) el.classList.add('active');
      if (step === -1 && n === 4) el.classList.add('active');
    });
  }

  function renderBlueDeployUI() {
    const rel = state.currentBlue;
    const step = currentStep(rel);
    let cfg = BLUE_STEPS[step] || BLUE_STEPS[4];
    const inProgress = isDeployInProgress(rel);
    const prodBlocked = !isProductionGreen() && step === 4 && !inProgress && step !== 0;

    renderBlueStepper(step === 0 ? 4 : step);
    renderBlueGuard('blueDeploy');
    renderBlueDeployLock();

    $('blueActionForm').style.display = step === 1 && cfg.showForm ? 'block' : 'none';
    $('blueSuccessBox').hidden = step !== 0;
    $('blueWaitingBox').hidden = !inProgress;
    $('blueMainBtn').hidden = step === 0 || step === -1 || inProgress || (step === 4 && isOtherDeployBusy(rel)) || prodBlocked;
    const resetBtn = $('btnNewBlueDeploy');
    if (resetBtn) resetBtn.hidden = !(step === 0 || step === -1) || isAnyDeployBusy();

    if (step === 0) {
      const auto = autoTestStep(rel);
      const testWarn = auto?.status === 'failed' || rel?.status === 'failed';
      if (resetBtn) resetBtn.textContent = '再部署一遍 →';
      $('blueStepBadge').textContent = '完成';
      $('blueActionTitle').textContent = testWarn ? '蓝环境已部署（测试有警告）' : '部署完成';
      $('blueActionDesc').textContent = testWarn
        ? '代码已成功部署到蓝环境。自动测试未完全通过，请手动打开 :58080 验收。'
        : `发布单「${rel?.title || ''}」已部署到蓝环境，可以去验收了。`;
      return;
    }

    if (step === -1) {
      $('blueStepBadge').textContent = '失败';
      $('blueActionTitle').textContent = '部署失败';
      $('blueActionDesc').textContent = rel?.steps?.find((s) => s.status === 'failed')?.message || '请展开下方日志查看原因。';
      $('blueMainBtn').hidden = true;
      if (resetBtn) resetBtn.textContent = '重新开始 →';
      return;
    }

    if (prodBlocked) {
      $('blueStepBadge').textContent = '第 4 步';
      $('blueActionTitle').textContent = '暂不可部署';
      $('blueActionDesc').textContent = productionGuardMessage() || '生产流量不在绿环境，请先切流后再部署蓝环境。';
      return;
    }

    if (step === 4 && isOtherDeployBusy(rel) && !inProgress) {
      $('blueStepBadge').textContent = '第 4 步';
      $('blueActionTitle').textContent = '暂不可部署';
      $('blueActionDesc').textContent = `发布单「${state.activeDeploy.title}」正在部署中，同一时间只能跑一个部署任务，请等待完成。`;
      return;
    }

    if (rel && inProgress) {
      const deploy = deployStep(rel);
      const reverting = isRevertingDeploy(rel);
      const cancelBtn = $('btnCancelBlueDeploy');
      if (cancelBtn) cancelBtn.hidden = reverting;
      if (reverting) {
        $('blueStepBadge').textContent = '第 4 步';
        $('blueActionTitle').textContent = '正在终止部署…';
        $('blueActionDesc').textContent = '正在取消 GitHub Actions 并回滚到部署前版本，请稍候。';
        $('blueWaitingText').textContent = deploy?.message || '正在终止并回滚…（约 3–8 分钟）';
        return;
      }
      if (deploy?.status === 'success') {
        $('blueStepBadge').textContent = '第 4 步';
        $('blueActionTitle').textContent = '正在自动测试…';
        $('blueActionDesc').textContent = '蓝环境代码已就绪，正在跑自动测试，请稍候。';
        $('blueWaitingText').textContent = '自动测试进行中…';
        return;
      }
      $('blueStepBadge').textContent = '第 4 步';
      $('blueActionTitle').textContent = rel.status === 'deploying' ? '正在部署…' : '正在自动测试…';
      $('blueActionDesc').textContent = '无需操作，等待即可。';
      $('blueWaitingText').textContent = rel.status === 'deploying'
        ? 'GitHub Actions 正在跑前后端 workflow…（约 3–8 分钟）'
        : '蓝环境已就绪，正在跑自动测试…';
      return;
    }
    const cancelBlueBtn = $('btnCancelBlueDeploy');
    if (cancelBlueBtn) cancelBlueBtn.hidden = true;

    if (step === 3 && rel && reviewsOK(rel.items?.[0]) && !rel.boss_approved) {
      $('blueStepBadge').textContent = '第 3 步';
      $('blueActionForm').style.display = 'none';
      if (isBoss()) {
        $('blueActionTitle').textContent = '终审发布';
        $('blueActionDesc').textContent = '双评审已通过，请确认终审后进入部署。';
        $('blueMainBtnText').textContent = '终审通过 →';
        $('blueMainBtn').hidden = false;
      } else {
        $('blueActionTitle').textContent = '等待终审';
        $('blueActionDesc').textContent = `双评审已通过，需 ${state.health?.deploy?.boss_reviewer || 'juege'} 登录终审后才能部署。`;
        $('blueMainBtn').hidden = true;
      }
      return;
    }

    if (step === 1 && isAdmin()) {
      cfg = { ...cfg, desc: '管理员账号可直接创建并部署，无需双评审与终审。' };
    }

    $('blueStepBadge').textContent = cfg.badge;
    $('blueActionTitle').textContent = cfg.title;
    $('blueActionDesc').textContent = cfg.desc;
    $('blueMainBtnText').textContent = cfg.btn;
  }

  function renderBlueLogs(rel) {
    const steps = rel?.steps || [];
    $('blueSteps').innerHTML = steps.length
      ? steps.map((s) => `
        <li>
          <span class="dot ${s.status}"></span>
          <div>
            <div class="step-title">${escapeHtml(s.title)}</div>
            <div class="step-msg">${escapeHtml(s.status)} ${escapeHtml(s.message || '')}</div>
          </div>
        </li>`).join('')
      : '<li><div class="empty">暂无日志，完成第 1 步后这里会显示进度</div></li>';
  }

  async function loadBlueList() {
    const list = filterByTarget(await api('/api/releases'), 'blue');
    const el = $('blueList');
    if (!list.length) {
      el.innerHTML = '<div class="empty">还没有蓝环境发布记录</div>';
      return;
    }
    el.innerHTML = list.map((r) => `
      <button type="button" class="release-item ${state.currentBlue?.id === r.id ? 'active' : ''}" data-id="${r.id}">
        <strong>${escapeHtml(r.title)}</strong>
        <span class="meta">${escapeHtml(r.status)} · ${new Date(r.updated_at).toLocaleString()}</span>
      </button>`).join('');
    el.querySelectorAll('.release-item').forEach((node) => {
      node.addEventListener('click', () => selectBlue(node.dataset.id));
    });
  }

  async function selectBlue(id) {
    state.currentBlue = await api(`/api/releases/${id}`);
    renderBlueDeployUI();
    renderBlueLogs(state.currentBlue);
    await loadBlueList();
    scheduleBlueDeployPoll();
    toast('已切换到：' + state.currentBlue.title);
  }

  function resetBlueDeploy() {
    if (isAnyDeployBusy()) {
      toast('有部署任务进行中，请等待完成后再新建', 'error');
      return;
    }
    if (state.blueDeployPollTimer) {
      clearInterval(state.blueDeployPollTimer);
      state.blueDeployPollTimer = null;
    }
    state.currentBlue = null;
    $('blueTitle').value = '';
    $('blueLogFold').open = false;
    renderBlueDeployUI();
    renderBlueLogs(null);
    loadBlueList();
    toast('已重置，请填写新的发布名称开始蓝环境部署');
  }

  async function autoPickBlueRelease(list) {
    const blueList = filterByTarget(list, 'blue');
    const active = blueList.find((r) => {
      const step = currentStep(r);
      return step > 0 || isDeployInProgress(r);
    });
    if (active) await selectBlue(active.id);
  }

  async function createBlueRelease() {
    const title = $('blueTitle').value.trim();
    if (!title) throw new Error('请填写发布名称');
    const author = $('blueAuthor').value.trim();
    if (!author) throw new Error('请填写你的名字');
    const rel = await api('/api/releases', {
      method: 'POST',
      body: JSON.stringify({
        title,
        commit_sha: 'blue-deploy',
        author,
        deploy_target: 'blue',
        level: 'normal',
        repo: 'juege-osh/osh',
        items: [{
          title: '前后端蓝环境部署',
          type: 'code',
          ref: 'deploy-prod',
          developer: $('developer').value,
          expected_impact: '同步蓝环境代码，不影响生产',
          reviewer1: $('rev1').value,
          reviewer2: $('rev2').value,
        }],
      }),
    });
    state.currentBlue = rel;
    toast('第 1 步完成！继续点蓝色按钮');
  }

  async function submitBlueReview() {
    const actor = $('blueAuthor').value.trim() || 'ops';
    state.currentBlue = await api(`/api/releases/${state.currentBlue.id}/submit-review`, {
      method: 'POST',
      body: JSON.stringify({ actor }),
    });
    toast('第 2 步完成！继续点蓝色按钮');
  }

  async function completeBlueApproval() {
    const item = state.currentBlue.items[0];
    if (!reviewsOK(item)) {
      for (const reviewer of [item.reviewer1, item.reviewer2]) {
        await api(`/api/items/${item.id}/reviews`, {
          method: 'POST',
          body: JSON.stringify({
            reviewer, tested: true,
            demo_seen: reviewer !== item.developer,
            result: 'approve', comment: '通过',
          }),
        });
      }
    }
    state.currentBlue = await api(`/api/releases/${state.currentBlue.id}`);
    if (isBoss()) {
      state.currentBlue = await api(`/api/releases/${state.currentBlue.id}/boss-approve`, {
        method: 'POST',
        body: JSON.stringify({ reviewer: state.user.username, comment: '终审通过' }),
      });
      toast('终审通过，可以部署了');
    } else {
      toast('双评审已提交，等待 juege 终审');
    }
  }

  async function bossApproveBlueOnly() {
    state.currentBlue = await api(`/api/releases/${state.currentBlue.id}/boss-approve`, {
      method: 'POST',
      body: JSON.stringify({ reviewer: state.user.username, comment: '终审通过' }),
    });
    toast('终审通过，可以部署了');
  }

  async function deployBlueRelease() {
    if (isOtherDeployBusy(state.currentBlue)) {
      throw new Error(`发布单「${state.activeDeploy.title}」正在部署中，请等待完成`);
    }
    if (!isProductionGreen()) {
      throw new Error(productionGuardMessage() || '生产流量不在绿环境');
    }
    const actor = $('blueAuthor').value.trim() || 'ops';
    state.currentBlue = { ...state.currentBlue, status: 'deploying' };
    state.activeDeploy = {
      busy: true,
      id: state.currentBlue.id,
      title: state.currentBlue.title,
      status: 'deploying',
    };
    renderBlueDeployUI();
    renderBlueDeployLock();

    const rel = await api(`/api/releases/${state.currentBlue.id}/deploy`, {
      method: 'POST',
      body: JSON.stringify({ actor }),
    });
    state.currentBlue = rel;
    toast('已触发蓝环境部署，正在等待 GitHub Actions…');
    $('blueLogFold').open = true;
    await loadActiveDeploy();
    scheduleBlueDeployPoll();
  }

  async function handleBlueMainAction() {
    if (state.busy) return;
    state.busy = true;
    $('loading').classList.add('show');
    $('blueMainBtn').disabled = true;
    try {
      const step = currentStep(state.currentBlue);
      if (step === 1) await createBlueRelease();
      else if (step === 2) await submitBlueReview();
      else if (step === 3) {
        if (reviewsOK(state.currentBlue.items?.[0]) && isBoss()) await bossApproveBlueOnly();
        else await completeBlueApproval();
      }
      else if (step === 4) {
        $('loading').classList.remove('show');
        await deployBlueRelease();
      }
      renderBlueDeployUI();
      renderBlueLogs(state.currentBlue);
      await loadBlueList();
      if (currentStep(state.currentBlue) > 1 && currentStep(state.currentBlue) < 5) {
        $('blueLogFold').open = true;
      }
    } catch (err) {
      toast(err.message || String(err), 'error');
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      $('blueMainBtn').disabled = false;
      renderBlueUI();
    }
  }

  function scheduleBlueDeployPoll() {
    if (state.blueDeployPollTimer) clearInterval(state.blueDeployPollTimer);
    state.blueDeployPollTimer = null;
    if (!state.currentBlue) return;
    const step = currentStep(state.currentBlue);
    if (!isDeployInProgress(state.currentBlue) && step !== 4) return;
    state.blueDeployPollTimer = setInterval(async () => {
      try {
        if (!state.currentBlue) return;
        state.currentBlue = await api(`/api/releases/${state.currentBlue.id}`);
        renderBlueDeployUI();
        renderBlueLogs(state.currentBlue);
        await loadBlueList();
        await loadActiveDeploy();
        if (!isDeployInProgress(state.currentBlue)) {
          clearInterval(state.blueDeployPollTimer);
          state.blueDeployPollTimer = null;
          const step = currentStep(state.currentBlue);
          if (step === 0) toast('蓝环境部署完成！请打开 :58080 验收');
          else if (state.currentBlue.status === 'failed') toast('蓝环境部署失败，请查看日志', 'error');
        }
      } catch { /* ignore */ }
    }, 3000);
  }

  function scheduleBluePoll() {
    if (state.bluePollTimer) clearInterval(state.bluePollTimer);
    state.bluePollTimer = null;
    if (!isBlueBusy()) return;
    state.bluePollTimer = setInterval(async () => {
      await loadBlueActive();
      if (!isBlueBusy()) {
        clearInterval(state.bluePollTimer);
        state.bluePollTimer = null;
        const job = state.blueActive?.job;
        if (job?.status === 'success') toast(job.message || '蓝环境任务完成');
        else if (job?.status === 'failed') toast(job.message || '蓝环境任务失败', 'error');
      }
    }, 4000);
  }

  async function executeBlueCustomSql() {
    if (!canOperateBlueSql()) {
      toast(productionGuardMessage() || '当前不可对蓝库执行 SQL', 'error');
      return;
    }
    if (state.busy) return;
    const sql = $('blueCustomSql')?.value.trim();
    if (!sql) { toast('请先填写 SQL', 'error'); return; }
    const actor = $('author')?.value.trim() || 'ops';
    const label = $('blueSqlLabel')?.value.trim() || 'custom';
    if (!confirm('确认将以下 SQL 执行到蓝环境 MySQL？\n\n目标：osh-mysql / backstage（待命库）\n\n执行前会生成快照；失败会自动尝试恢复。')) return;

    state.busy = true;
    $('loading')?.classList.add('show');
    const btn = $('btnExecBlueSql');
    if (btn) btn.disabled = true;
    const resultEl = $('blueSqlResult');
    if (resultEl) resultEl.hidden = true;
    try {
      const res = await api('/api/blue/sql/execute', {
        method: 'POST',
        body: JSON.stringify({ sql, actor, label }),
      });
      if (resultEl) {
        resultEl.hidden = false;
        resultEl.textContent = formatSqlResult(res, '✓ 执行成功');
        resultEl.className = 'sql-result ok';
      }
      toast('SQL 已执行到蓝库');
    } catch (err) {
      if (resultEl) {
        resultEl.hidden = false;
        resultEl.textContent = formatSqlFailure(err);
        resultEl.className = 'sql-result err';
      }
      toast(formatSqlError(err.message || String(err)).split('\n')[0], 'error');
    } finally {
      state.busy = false;
      $('loading')?.classList.remove('show');
      renderBlueSqlUI();
    }
  }

  async function syncBlue() {
    if (!canOperateBlue()) {
      toast(productionGuardMessage() || blueHintText(), 'error');
      return;
    }
    const actor = $('author').value.trim() || 'ops';
    const reason = $('blueSyncReason').value.trim();
    if (!confirm('确认执行绿→蓝数据同步？\n\n将同步 MySQL、Nacos、ES、Redis 到蓝环境待命库，耗时约 5–20 分钟。\n\n前提：当前生产流量须在绿环境。')) return;

    state.busy = true;
    $('loading').classList.add('show');
    const resultEl = $('blueSyncResult');
    resultEl.hidden = true;
    renderBlueUI();
    try {
      const job = await api('/api/blue/sync', {
        method: 'POST',
        body: JSON.stringify({ actor, reason }),
      });
      state.blueActive = { busy: true, job };
      toast('已启动绿→蓝数据同步…');
      renderBlueUI();
      scheduleBluePoll();
      await loadBlueActive();
    } catch (err) {
      toast(err.message || String(err), 'error');
      renderBlueUI();
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      renderBlueUI();
    }
  }

  async function syncGreenAllComponents() {
    if (!canOperateGreenSync()) {
      toast(greenSyncHintText(), 'error');
      return;
    }
    const actor = state.user?.username || $('author')?.value?.trim() || 'ops';
    const reason = $('greenSyncReason')?.value?.trim() || '';
    if (!confirm('确认执行蓝→绿环境基线同步？\n\n只读蓝环境，写入绿环境；不会切流，不会操作蓝环境。真正上线请走 Change 作战台。')) return;

    state.busy = true;
    $('loading').classList.add('show');
    const resultEl = $('greenSyncResult');
    if (resultEl) resultEl.hidden = true;
    renderGreenSyncUI();
    try {
      const job = await api('/api/component-sync/blue-to-green/all', {
        method: 'POST',
        body: JSON.stringify({ actor, reason }),
      });
      state.componentSyncActive = { busy: true, job };
      toast('已启动蓝→绿环境基线同步…');
      renderGreenSyncUI();
      scheduleComponentSyncPoll();
      await loadComponentSyncActive();
    } catch (err) {
      toast(err.message || String(err), 'error');
      renderGreenSyncUI();
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      renderGreenSyncUI();
    }
  }

  async function loadActiveDeploy() {
    try {
      const d = await api('/api/deploy/active');
      state.activeDeploy = d.busy ? d : null;
    } catch {
      state.activeDeploy = null;
    }
    renderDeployLock();
    renderBlueDeployLock();
    scheduleActivePoll();
  }

  function renderDeployLock() {
    const banner = $('deployLockBanner');
    const text = $('deployLockText');
    if (!banner || !text) return;

    if (!isAnyDeployBusy()) {
      banner.hidden = true;
      return;
    }

    const d = state.activeDeploy;
    const isOther = isOtherDeployBusy(state.current);
    banner.hidden = false;
    banner.classList.toggle('self', !isOther);
    text.textContent = isOther
      ? `发布单「${d.title}」正在部署中，请等待完成后再发起新的部署`
      : `当前发布单正在部署中，请等待完成…`;
  }

  function scheduleActivePoll() {
    if (state.activePollTimer) clearInterval(state.activePollTimer);
    state.activePollTimer = null;
    if (!isAnyDeployBusy()) return;
    state.activePollTimer = setInterval(async () => {
      await loadActiveDeploy();
      if (!isAnyDeployBusy()) {
        clearInterval(state.activePollTimer);
        state.activePollTimer = null;
        renderUI();
        renderBlueDeployUI();
      }
    }, 4000);
  }

  async function api(path, opts = {}) {
    const res = await fetch(path, {
      ...opts,
      headers: { ...authHeaders(), ...(opts.headers || {}) },
    });
    let data = {};
    try { data = await res.json(); } catch { /* empty */ }
    if (!res.ok) {
      const err = new Error(data.error || res.statusText);
      err.data = data;
      throw err;
    }
    return data;
  }

  function toast(msg, type = 'success') {
    const el = document.createElement('div');
    el.className = `toast ${type}`;
    el.textContent = msg;
    $('toasts').appendChild(el);
    setTimeout(() => el.remove(), 4000);
  }

  function escapeHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function warValue(id) {
    return ($(id)?.value || '').trim();
  }

  function activeWarComponent() {
    return WAR_COMPONENTS[state.warRoom.activeComponent] || WAR_COMPONENTS.mysql;
  }

  function setPlaceholder(id, text) {
    const el = $(id);
    if (el) el.placeholder = text || '';
  }

  function renderWarComponentPicker() {
    const tpl = activeWarComponent();
    document.querySelectorAll('[data-component-tab]').forEach((btn) => {
      const active = btn.dataset.componentTab === state.warRoom.activeComponent;
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    const select = $('componentType');
    if (select) select.value = state.warRoom.activeComponent;
    const name = $('activeComponentName');
    if (name) name.textContent = tpl.label;
    const summary = $('activeComponentSummary');
    if (summary) summary.textContent = tpl.summary;
    const guide = $('warComponentGuide');
    if (guide) {
      guide.innerHTML = `<strong>${escapeHtml(tpl.label)} 上线怎么填</strong><span>${escapeHtml(tpl.guide)}</span>`;
    }
    const changeType = $('changeType');
    if (changeType) changeType.value = tpl.type;
    const action = $('changeAction');
    if (action && (!action.value.trim() || action.dataset.templateValue === action.value)) {
      action.value = tpl.action;
    }
    if (action) action.dataset.templateValue = tpl.action;
    setPlaceholder('changeTitle', tpl.title);
    setPlaceholder('changeRef', tpl.ref);
    setPlaceholder('targetNode', tpl.node);
    setPlaceholder('changeAction', tpl.action);
    setPlaceholder('expectedImpact', tpl.impact);
    setPlaceholder('dataImpact', tpl.data);
    setPlaceholder('testPlan', tpl.test);
    setPlaceholder('rollbackStrategy', tpl.rollback);
  }

  function selectWarComponent(kind) {
    if (!WAR_COMPONENTS[kind]) return;
    state.warRoom.activeComponent = kind;
    renderWarComponentPicker();
  }

  async function loadWarRoom() {
    renderWarRoom();
    renderWarComponentPicker();
    try {
      state.warRoom.specs = await api('/api/components/specs');
    } catch {
      state.warRoom.specs = [];
    }
    if (state.current?.id) {
      try {
        const [executions, reports, conflicts] = await Promise.all([
          api(`/api/releases/${state.current.id}/executions`),
          api(`/api/releases/${state.current.id}/component-reports`),
          api(`/api/releases/${state.current.id}/conflicts`),
        ]);
        state.warRoom.executions = executions;
        state.warRoom.reports = reports;
        state.warRoom.conflicts = conflicts;
      } catch {
        state.warRoom.executions = [];
        state.warRoom.reports = [];
        state.warRoom.conflicts = [];
      }
    }
    renderWarRoom();
    renderComponentSpecs();
    renderWarReports();
  }

  function addWarItem() {
    const title = warValue('changeTitle');
    if (!title) {
      toast('请填写 Change 标题', 'error');
      return;
    }
    const reviewers = warValue('changeReviewers').split(',').map((v) => v.trim()).filter(Boolean);
    if (reviewers.length < 2) {
      toast('每个 change 必须填写两个评审人，用英文逗号分隔', 'error');
      return;
    }
    const componentType = state.warRoom.activeComponent || warValue('componentType') || 'application';
    const item = {
      title,
      type: warValue('changeType') || 'component',
      ref: warValue('changeRef') || title,
      developer: warValue('changeDeveloper') || state.user?.username || 'developer',
      expected_impact: warValue('expectedImpact'),
      component: componentType === 'application' ? 'application' : componentType,
      component_type: componentType,
      action: warValue('changeAction') || 'apply',
      target_slot: 'green',
      target_node: warValue('targetNode') || 'all',
      deploy_order: Number(warValue('warDeployOrder') || 100),
      impact_scope: warValue('expectedImpact'),
      data_impact: warValue('dataImpact'),
      test_plan: warValue('testPlan'),
      rollback_strategy: warValue('rollbackStrategy'),
      conflict_owners: warValue('conflictOwners'),
      notify_emails: warValue('notifyEmails'),
      reviewer1: reviewers[0],
      reviewer2: reviewers[1],
    };
    state.warRoom.items.push(item);
    ['changeTitle', 'changeRef', 'expectedImpact', 'dataImpact', 'testPlan', 'rollbackStrategy'].forEach((id) => {
      if ($(id)) $(id).value = '';
    });
    const action = $('changeAction');
    if (action) action.value = activeWarComponent().action;
    renderWarRoom();
    renderWarComponentPicker();
    toast('已加入 change 列表');
  }

  async function createWarRelease() {
    const title = warValue('warReleaseTitle');
    if (!title) {
      toast('请填写发布名称', 'error');
      return;
    }
    if (!state.warRoom.items.length) {
      toast('请至少加入一个 change', 'error');
      return;
    }
    const rel = await api('/api/releases', {
      method: 'POST',
      body: JSON.stringify({
        title,
        commit_sha: 'change-driven-release',
        author: state.user?.username || warValue('changeDeveloper') || 'ops',
        level: warValue('warLevel') || 'normal',
        repo: 'juege-osh/osh',
        deploy_target: 'green',
        items: state.warRoom.items,
      }),
    });
    state.current = rel;
    state.warRoom.items = [];
    await recordWarConflicts(rel);
    toast('发布单已生成，请进入部署绿环境继续评审和上线');
    renderWarRoom();
    renderUI();
    loadList();
    navigate('deploy');
  }

  async function recordWarConflicts(rel) {
    if (!rel?.id) return;
    for (const item of rel.items || []) {
      const owners = (item.conflict_owners || '').split(',').map((v) => v.trim()).filter(Boolean);
      const emails = (item.notify_emails || '').split(',').map((v) => v.trim()).filter(Boolean);
      for (let i = 0; i < owners.length; i += 1) {
        await api(`/api/releases/${rel.id}/conflicts`, {
          method: 'POST',
          body: JSON.stringify({
            item_id: item.id,
            file_path: item.ref || item.title,
            owner: owners[i],
            email: emails[i] || '',
            status: emails[i] ? 'pending' : 'audit_only',
            message: 'Change 作战台记录冲突责任人；SMTP 未配置时仅审计不阻塞发布。',
          }),
        });
      }
    }
  }

  function renderWarRoom() {
    const root = $('warRoomView');
    if (!root) return;
    document.querySelectorAll('[data-war-view]').forEach((btn) => {
      btn.classList.toggle('active', btn.dataset.warView === state.warRoom.view);
    });
    const items = state.warRoom.items.length ? state.warRoom.items : (state.current?.items || []);
    if (!items.length) {
      root.innerHTML = `
        <div class="war-empty">
          <strong>还没有上线项</strong>
          <span>按左侧 3 步来：填发布名称 → 选组件 → 填 change，然后点“加入 change 列表”。</span>
          <span>多个组件可以一条一条加入，平台会按上线顺序执行。</span>
        </div>`;
      renderWarReports();
      return;
    }
    if (state.warRoom.view === 'tree') {
      root.innerHTML = items.map((it) => `
        <div class="tree-node">
          <strong>${escapeHtml(it.component || it.component_type || 'application')} · #${escapeHtml(it.deploy_order || 100)}</strong>
          <span>${escapeHtml(it.title)} → ${escapeHtml(it.target_node || 'all')}</span>
          <span>测试：${escapeHtml(it.test_plan || '未填写')}</span>
          <span>回滚：${escapeHtml(it.rollback_strategy || '未填写')}</span>
        </div>`).join('');
      return;
    }
    if (state.warRoom.view === 'graph') {
      root.innerHTML = `<div class="graph-flow">${items.map((it, i) => `
        <div class="graph-node">
          <strong>${escapeHtml(it.component || it.component_type || 'application')}</strong>
          <span>${escapeHtml(it.title)}</span>
          <span>${escapeHtml(it.target_node || 'all')}</span>
        </div>${i < items.length - 1 ? '<span class="graph-arrow">→</span>' : ''}`).join('')}</div>`;
      return;
    }
    root.innerHTML = `
      <table class="war-table">
        <thead><tr><th>顺序</th><th>组件</th><th>Change</th><th>节点</th><th>影响/测试</th><th>责任</th></tr></thead>
        <tbody>${items.map((it) => `
          <tr>
            <td>${escapeHtml(it.deploy_order || 100)}</td>
            <td>${escapeHtml(it.component_type || it.component || 'application')}</td>
            <td><strong>${escapeHtml(it.title)}</strong><br><span>${escapeHtml(it.ref || '')}</span></td>
            <td>${escapeHtml(it.target_node || 'all')}</td>
            <td>${escapeHtml(it.expected_impact || '未填写')}<br><span>${escapeHtml(it.test_plan || '未填写测试计划')}</span></td>
            <td>${escapeHtml(it.developer || '')}<br><span>${escapeHtml(it.reviewer1 || '')} / ${escapeHtml(it.reviewer2 || '')}</span></td>
          </tr>`).join('')}</tbody>
      </table>`;
  }

  function renderComponentSpecs() {
    const el = $('componentSpecs');
    if (!el) return;
    if (!state.warRoom.specs.length) {
      el.innerHTML = '<div class="empty">暂无组件规范模板</div>';
      return;
    }
    el.innerHTML = state.warRoom.specs.map((sp) => `
      <div class="spec-card">
        <strong>${escapeHtml(sp.kind)} · ${escapeHtml(sp.name)}</strong>
        <span>数据目录：${escapeHtml(sp.data_dir || '待定义')}</span>
        <span>配置目录：${escapeHtml(sp.config_dir || '待定义')}</span>
        <span>健康检查：${escapeHtml(sp.health_check || '待定义')}</span>
        <span>回滚：${escapeHtml(sp.rollback_strategy || '待定义')}</span>
      </div>`).join('');
  }

  function renderWarReports() {
    const el = $('warReports');
    if (!el) return;
    const executions = state.warRoom.executions || [];
    const reports = state.warRoom.reports || [];
    const conflicts = state.warRoom.conflicts || [];
    if (!executions.length && !reports.length && !conflicts.length) {
      el.innerHTML = '<div class="empty">暂无执行记录；发布单跑到组件执行阶段后会显示。</div>';
      return;
    }
    el.innerHTML = [
      ...executions.map((e) => `
        <div class="report-card"><strong>${escapeHtml(e.slot)} · ${escapeHtml(e.component)}</strong><span>${escapeHtml(e.status)} · ${escapeHtml(e.action)}</span><span>${escapeHtml(e.error || e.output || '已记录')}</span>${e.item_id ? `<div class="report-actions"><button type="button" class="mini-danger" data-rollback-item="${escapeHtml(e.item_id)}" data-rollback-slot="${escapeHtml(e.slot || 'green')}">回滚${escapeHtml(e.slot || 'green')}</button></div>` : ''}</div>`),
      ...reports.map((r) => {
        const mini = renderAutoTestReportHTML({
          functional_json: r.functional_json,
          data_diff_json: r.data_diff_json,
          ai_verdict: r.ai_verdict,
          passed: r.passed,
        });
        return `
        <div class="report-card report-card-wide">
          <strong>组件测试 ${escapeHtml(r.slot)} · ${escapeHtml(r.component)}</strong>
          <span>${r.passed ? '通过' : '失败'} · ${escapeHtml(r.ai_verdict || '')}</span>
          <div class="auto-test-body compact">${mini}</div>
        </div>`;
      }),
      ...conflicts.map((c) => `
        <div class="report-card"><strong>冲突通知 · ${escapeHtml(c.owner)}</strong><span>${escapeHtml(c.file_path)}</span><span>${escapeHtml(c.status)} ${escapeHtml(c.email || '')}</span></div>`),
    ].join('');
    el.querySelectorAll('[data-rollback-item]').forEach((btn) => {
      btn.addEventListener('click', () => rollbackWarItem(btn.dataset.rollbackItem, btn.dataset.rollbackSlot || 'green'));
    });
  }

  async function rollbackWarItem(itemId, slot = 'green') {
    if (!itemId || state.busy) return;
    if (!confirm(`确认回滚该上线项的 ${slot} 环境变更？`)) return;
    state.busy = true;
    try {
      await api(`/api/items/${itemId}/rollback`, {
        method: 'POST',
        body: JSON.stringify({ slot, actor: state.user?.username || warValue('changeDeveloper') || 'ops' }),
      });
      toast('组件回滚已执行');
      await loadWarRoom();
    } catch (err) {
      toast(err.message || String(err), 'error');
    } finally {
      state.busy = false;
    }
  }

  function reviewsOK(item) {
    if (!item) return false;
    const need = new Set([item.reviewer1, item.reviewer2].filter(Boolean));
    const ok = new Set();
    for (const rv of item.reviews || []) {
      if (rv.result === 'approve' && rv.tested) ok.add(rv.reviewer);
    }
    return need.size === 2 && [...need].every((r) => ok.has(r));
  }

  function allReviewsOK(items) {
    return (items || []).length > 0 && (items || []).every((item) => reviewsOK(item));
  }

  function autoTestStep(rel) {
    return (rel?.steps || []).find((s) => s.step_key === 'auto_test');
  }

  function deployStep(rel) {
    return (rel?.steps || []).find((s) => s.step_key === 'deploy_standby');
  }

  function isRevertingDeploy(rel) {
    const deploy = deployStep(rel);
    return deploy?.status === 'running' && (deploy?.message || '').includes('终止');
  }

  /** 当前应该做第几步（1-4），0=已完成，-1=失败 */
  function currentStep(rel) {
    if (!rel) return 1;
    const deploy = deployStep(rel);
    const auto = autoTestStep(rel);

    if (rel.status === 'done') return 0;
    if (deploy?.status === 'success' && auto?.status === 'success') return 0;
    if (rel.status === 'testing' && auto?.status === 'success') return 0;

    if (rel.status === 'failed') {
      if (deploy?.status === 'success') return 0; // 绿环境已部署，仅测试/旁路失败
      return -1;
    }

    if (['deploying', 'testing'].includes(rel.status)) {
      if (deploy?.status === 'success' && auto?.status !== 'success') return 4;
      return 4;
    }

    if (rel.boss_approved && allReviewsOK(rel.items)) return 4;
    if (isAdmin() && rel.boss_approved && ['draft', 'approved'].includes(rel.status)) return 4;
    if (allReviewsOK(rel.items) && !rel.boss_approved) return 3;
    if (rel.status === 'draft') return isAdmin() && rel.boss_approved ? 4 : 2;
    if (rel.status === 'reviewing') return 3;
    return 2;
  }

  function renderStepper(step) {
    document.querySelectorAll('#stepper .stepper-item, #pageDeploy .stepper-item').forEach((el) => {
      const n = Number(el.dataset.step);
      el.classList.remove('active', 'done');
      const adminSkip = isAdmin() && step === 4 && (n === 2 || n === 3);
      if (step === 0 || (step > 0 && n < step) || adminSkip) el.classList.add('done');
      if (n === step) el.classList.add('active');
      if (step === -1 && n === 4) el.classList.add('active');
    });
  }

  function isDeployInProgress(rel) {
    if (!rel) return false;
    if (currentStep(rel) !== 4) return false;
    const deploy = deployStep(rel);
    const auto = autoTestStep(rel);
    if (rel.status === 'deploying' && deploy?.status !== 'success') return true;
    if (rel.status === 'testing' && deploy?.status === 'success' && auto?.status !== 'success') return true;
    if (deploy?.status === 'running' || auto?.status === 'running') return true;
    return false;
  }

  function renderUI() {
    const rel = state.current;
    const step = currentStep(rel);
    let cfg = STEPS[step] || STEPS[4];
    const inProgress = isDeployInProgress(rel);

    renderStepper(step === 0 ? 4 : step);

    $('actionForm').style.display = step === 1 && cfg.showForm ? 'block' : 'none';
    $('successBox').hidden = step !== 0;
    $('waitingBox').hidden = !inProgress;
    $('mainBtn').hidden = step === 0 || step === -1 || inProgress || (step === 4 && isOtherDeployBusy(rel));
    const resetBtn = $('btnNewDeploy');
    if (resetBtn) resetBtn.hidden = !(step === 0 || step === -1) || isAnyDeployBusy();

    if (step === 0) {
      const deploy = deployStep(rel);
      const auto = autoTestStep(rel);
      const testWarn = auto?.status === 'failed' || rel?.status === 'failed';
      loadDeployAutoTestReport(rel?.id);
      if (resetBtn) resetBtn.textContent = '再部署一遍 →';
      $('stepBadge').textContent = '完成';
      $('actionTitle').textContent = testWarn ? '绿环境已部署（测试有警告）' : '部署完成';
      $('actionDesc').textContent = testWarn
        ? '代码已成功部署到绿环境。自动测试未完全通过，请手动打开绿环境验收。'
        : `发布单「${rel?.title || ''}」已部署到绿环境，可以去验收了。`;
      return;
    }

    if (step === -1) {
      $('stepBadge').textContent = '失败';
      $('actionTitle').textContent = '部署失败';
      $('actionDesc').textContent = rel?.steps?.find((s) => s.status === 'failed')?.message || '请展开下方日志查看原因，或到 GitHub Actions 查日志。';
      $('mainBtn').hidden = true;
      if (resetBtn) resetBtn.textContent = '重新开始 →';
      return;
    }

    if (step === 4 && isOtherDeployBusy(rel) && !inProgress) {
      $('stepBadge').textContent = '第 4 步';
      $('actionTitle').textContent = '暂不可部署';
      $('actionDesc').textContent = `发布单「${state.activeDeploy.title}」正在部署中，同一时间只能跑一个部署任务，请等待完成。`;
      return;
    }

    if (rel && inProgress) {
      const deploy = deployStep(rel);
      const reverting = isRevertingDeploy(rel);
      const cancelBtn = $('btnCancelDeploy');
      if (cancelBtn) cancelBtn.hidden = reverting;
      if (reverting) {
        $('stepBadge').textContent = '第 4 步';
        $('actionTitle').textContent = '正在终止部署…';
        $('actionDesc').textContent = '正在取消 GitHub Actions 并回滚到部署前版本，请稍候。';
        $('waitingText').textContent = deploy?.message || '正在终止并回滚…（约 3–8 分钟）';
        return;
      }
      if (deploy?.status === 'success') {
        $('stepBadge').textContent = '第 4 步';
        $('actionTitle').textContent = '正在自动测试…';
        $('actionDesc').textContent = '绿环境代码已就绪，正在跑自动测试，请稍候。';
        $('waitingText').textContent = '自动测试进行中…（analyzer 未启动时会自动跳过）';
        return;
      }
      $('stepBadge').textContent = '第 4 步';
      $('actionTitle').textContent = rel.status === 'deploying' ? '正在部署…' : '正在自动测试…';
      $('actionDesc').textContent = '无需操作，等待即可。';
      $('waitingText').textContent = rel.status === 'deploying'
        ? 'GitHub Actions 正在跑前后端 workflow…（约 3–8 分钟，跑完才会显示成功）'
        : '绿环境已就绪，正在跑自动测试…';
      return;
    }
    const cancelBtn = $('btnCancelDeploy');
    if (cancelBtn) cancelBtn.hidden = true;

    if (step === 3 && rel && reviewsOK(rel.items?.[0]) && !rel.boss_approved) {
      $('stepBadge').textContent = '第 3 步';
      $('actionForm').style.display = 'none';
      if (isBoss()) {
        $('actionTitle').textContent = '终审发布';
        $('actionDesc').textContent = '双评审已通过，请确认终审后进入部署。';
        $('mainBtnText').textContent = '终审通过 →';
        $('mainBtn').hidden = false;
      } else {
        $('actionTitle').textContent = '等待终审';
        $('actionDesc').textContent = `双评审已通过，需 ${state.health?.deploy?.boss_reviewer || 'juege'} 登录终审后才能部署。`;
        $('mainBtn').hidden = true;
      }
      return;
    }

    if (step === 1 && isAdmin()) {
      cfg = { ...cfg, desc: '管理员账号可直接创建并部署，无需双评审与终审。' };
    }

    $('stepBadge').textContent = cfg.badge;
    $('actionTitle').textContent = cfg.title;
    $('actionDesc').textContent = cfg.desc;
    $('mainBtnText').textContent = cfg.btn;
    renderDeployLock();
    renderTrafficUI();
  }

  function renderLogs(rel) {
    const steps = rel?.steps || [];
    $('steps').innerHTML = steps.length
      ? steps.map((s) => `
        <li>
          <span class="dot ${s.status}"></span>
          <div>
            <div class="step-title">${escapeHtml(s.title)}</div>
            <div class="step-msg">${escapeHtml(s.status)} ${escapeHtml(s.message || '')}</div>
          </div>
        </li>`).join('')
      : '<li><div class="empty">暂无日志，完成第 1 步后这里会显示进度</div></li>';
  }

  async function loadHealth() {
    const h = await api('/api/health');
    state.health = h;
    const d = h.deploy || {};
    const mysql = h.mysql || {};
    $('modeBadge').textContent = h.mock_mode ? '演示模式' : '已连接';
    $('modeBadge').className = `badge ${h.mock_mode ? 'mock' : 'live'}`;
    $('ghaBadge').textContent = d.gha_enabled ? 'GitHub 部署' : 'GHA 未配置';
    $('ghaBadge').className = `badge ${d.gha_enabled ? 'live' : 'offline'}`;

    if (d.green_url) {
      $('greenLink').href = d.green_url;
      $('footerGreenLink').href = d.green_url;
      $('footerGreenLink').textContent = d.green_url;
    }
    const blueUrl = `http://${d.prod_host || '149.88.92.159'}:58080/`;
    const blueAccept = $('blueAcceptLink');
    if (blueAccept) blueAccept.href = blueUrl;
    $('footerBackend').textContent = d.backend_ref || '—';
    $('footerFrontend').textContent = d.frontend_ref || '—';

    const blueTip = $('blueDeployTip');
    if (blueTip) {
      blueTip.textContent = `目标：/opt/osh/app · 验收 http://${d.prod_host || '149.88.92.159'}:58080/ · 分支 ${d.backend_ref || '—'} / ${d.frontend_ref || '—'}`;
    }
    const blueSyncTip = $('blueSyncTip');
    if (blueSyncTip) {
      blueSyncTip.textContent = `方向：绿（生产）→ 蓝（待命）· 脚本 osh-prod-standby-sync.sh --green-to-blue`;
    }

    const tip = $('sqlTip');
    if (tip) {
      tip.textContent = mysql.configured
        ? `目标：${mysql.green_container || 'osh-g-mysql'} / ${mysql.green_database || 'backstage'} · 执行前快照，失败自动恢复`
        : '请在 config.env 配置 GREEN_MYSQL_ROOT_PASSWORD 后才能执行 SQL。';
    }
    const compTip = $('componentOpsTip');
    if (compTip) {
      compTip.textContent = mysql.configured
        ? `绿环境：${mysql.green_container || 'osh-g-mysql'} · 各组件执行前自动快照，失败可一键回滚`
        : '请先配置绿环境组件连接信息后再执行。';
    }
    const blueSqlTip = $('blueSqlTip');
    if (blueSqlTip) {
      blueSqlTip.textContent = mysql.configured
        ? `目标：${mysql.blue_container || 'osh-mysql'} / ${mysql.blue_database || 'backstage'} · 增量 SQL，执行前快照`
        : '请在 config.env 配置 BLUE_MYSQL_ROOT_PASSWORD（或与绿库相同）后才能执行。';
    }
  }

  async function loadSqlTemplates() {
    const sels = [$('sqlTemplate'), $('blueSqlTemplate')].filter(Boolean);
    if (!sels.length) return;
    try {
      const list = await api('/api/migrations');
      const options = '<option value="">从模板填入（可选）</option>' +
        list.map((m) => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.description || m.name)}</option>`).join('');
      sels.forEach((sel) => { sel.innerHTML = options; });
    } catch {
      sels.forEach((sel) => { sel.innerHTML = '<option value="">无可用模板</option>'; });
    }
  }

  async function loadTemplateIntoEditor() {
    const id = $('sqlTemplate').value;
    if (!id) { toast('请先选择模板', 'error'); return; }
    const data = await api(`/api/migrations/${id}`);
    $('customSql').value = data.sql || '';
    if (!$('sqlLabel').value.trim()) $('sqlLabel').value = id;
    toast('模板已填入，请检查后再执行');
  }

  const COMPONENT_BATCH_ORDER = { mysql: 10, nacos: 20, redis: 30, es: 40, kafka: 50 };

  const COMPONENT_TEST_CASES = {
    mysql: {
      label: '绿环境平台验收-MySQL',
      sql: `-- OSH 平台组件上线测试 · MySQL
USE backstage;
CREATE TABLE IF NOT EXISTS osh_platform_test (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  case_name VARCHAR(64) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO osh_platform_test (id, case_name) VALUES (1, 'platform-batch-test');`,
    },
    redis: {
      label: '绿环境平台验收-Redis',
      payload: 'SET osh:platform:test:key1 hello-from-platform EX 3600',
    },
    nacos: {
      label: '绿环境平台验收-Nacos',
      payload: `#!/usr/bin/env bash
set -euo pipefail
URL="http://127.0.0.1:28848"
curl -sf "$URL/nacos/v1/ns/operator/metrics" | head -c 120
echo
echo "nacos platform smoke ok"`,
    },
    es: {
      label: '绿环境平台验收-ES',
      payload: `#!/usr/bin/env bash
set -euo pipefail
ENV_FILE="/opt/osh-green/001-docker-compose/osh/osh-green-stack.env"
pass="$(awk -F= '$1=="ES_PASSWORD"{print substr($0,13);exit}' "$ENV_FILE")"
URL="http://127.0.0.1:29200"
INDEX="osh_platform_test"
curl -sf -u "elastic:\${pass}" -X PUT "$URL/$INDEX" -H 'Content-Type: application/json' \\
  -d '{"settings":{"number_of_shards":1,"number_of_replicas":0}}' || true
curl -sf -u "elastic:\${pass}" "$URL/_cluster/health" | head -c 120
echo
echo "es platform smoke ok"`,
    },
    kafka: {
      label: '绿环境平台验收-Kafka',
      topic: 'osh.platform.test.event',
      partitions: '1',
    },
  };

  function componentKindLabel(kind) {
    return ({ mysql: 'MySQL', redis: 'Redis', nacos: 'Nacos', es: 'ES', kafka: 'Kafka' })[kind] || kind;
  }

  function fillComponentTestCase(kind) {
    const tc = COMPONENT_TEST_CASES[kind];
    if (!tc) return;
    if (kind === 'mysql') {
      $('sqlLabel').value = tc.label;
      $('customSql').value = tc.sql;
    } else if (kind === 'redis') {
      $('redisLabel').value = tc.label;
      $('redisPayload').value = tc.payload;
    } else if (kind === 'nacos') {
      $('nacosLabel').value = tc.label;
      $('nacosRef').value = '';
      $('nacosPayload').value = tc.payload;
    } else if (kind === 'es') {
      $('esLabel').value = tc.label;
      $('esRef').value = '';
      $('esPayload').value = tc.payload;
    } else if (kind === 'kafka') {
      $('kafkaLabel').value = tc.label;
      $('kafkaTopic').value = tc.topic;
      $('kafkaPartitions').value = tc.partitions;
    }
    toast(`已填入 ${componentKindLabel(kind)} 测试用例，可「加入队列」或「立即执行」`);
  }

  function buildComponentQueueItem(kind) {
    if (kind === 'mysql') {
      const payload = $('customSql')?.value.trim();
      if (!payload) throw new Error('请先填写 MySQL SQL');
      return {
        kind: 'mysql',
        slot: 'green',
        action: 'apply',
        payload,
        label: warValue('sqlLabel') || 'mysql-batch',
        deploy_order: COMPONENT_BATCH_ORDER.mysql,
      };
    }
    if (kind === 'redis') {
      const payload = $('redisPayload')?.value.trim();
      if (!payload) throw new Error('请填写 Redis 命令');
      return {
        kind: 'redis',
        slot: 'green',
        action: 'apply',
        payload,
        label: warValue('redisLabel') || 'redis-batch',
        deploy_order: COMPONENT_BATCH_ORDER.redis,
      };
    }
    if (kind === 'nacos') {
      const ref = warValue('nacosRef');
      const payload = $('nacosPayload')?.value.trim();
      if (!ref && !payload) throw new Error('请填写 Nacos 脚本路径或内容');
      return {
        kind: 'nacos',
        slot: 'green',
        action: 'apply',
        ref,
        payload,
        label: warValue('nacosLabel') || 'nacos-batch',
        deploy_order: COMPONENT_BATCH_ORDER.nacos,
      };
    }
    if (kind === 'es') {
      const ref = warValue('esRef');
      const payload = $('esPayload')?.value.trim();
      if (!ref && !payload) throw new Error('请填写 ES 脚本路径或内容');
      return {
        kind: 'es',
        slot: 'green',
        action: 'apply',
        ref,
        payload,
        label: warValue('esLabel') || 'es-batch',
        deploy_order: COMPONENT_BATCH_ORDER.es,
      };
    }
    if (kind === 'kafka') {
      const topic = warValue('kafkaTopic');
      if (!topic) throw new Error('请填写 Topic 名称');
      return {
        kind: 'kafka',
        slot: 'green',
        action: 'create-topic',
        ref: topic,
        node: warValue('kafkaPartitions') || '3',
        label: warValue('kafkaLabel') || 'kafka-batch',
        deploy_order: COMPONENT_BATCH_ORDER.kafka,
      };
    }
    throw new Error(`未知组件: ${kind}`);
  }

  function queueComponentItem(kind) {
    const item = buildComponentQueueItem(kind);
    const dup = state.componentOpsQueue.some((q) => q.kind === item.kind && (q.ref || q.payload) === (item.ref || item.payload));
    if (dup) {
      toast(`${componentKindLabel(kind)} 相同内容已在队列中`, 'error');
      return;
    }
    state.componentOpsQueue.push(item);
    renderComponentOpsQueue();
    toast(`${componentKindLabel(kind)} 已加入队列（共 ${state.componentOpsQueue.length} 项）`);
  }

  function renderComponentOpsQueue() {
    const el = $('componentOpsQueue');
    if (!el) return;
    if (!state.componentOpsQueue.length) {
      el.innerHTML = '<div class="empty">队列为空。切换页签填写后点「加入队列」。</div>';
      return;
    }
    el.innerHTML = state.componentOpsQueue.map((item, idx) => {
      const summary = item.kind === 'kafka'
        ? `topic=${item.ref} · 分区=${item.node}`
        : (item.ref || (item.payload || '').split('\n')[0] || '').slice(0, 80);
      return `
        <div class="component-queue-item" data-idx="${idx}">
          <div class="component-queue-main">
            <strong>${escapeHtml(componentKindLabel(item.kind))}</strong>
            <span class="component-queue-order">顺序 ${item.deploy_order || '—'}</span>
            <span class="component-queue-label">${escapeHtml(item.label || '')}</span>
            <code class="component-queue-preview">${escapeHtml(summary)}</code>
          </div>
          <button type="button" class="btn btn-ghost btn-queue-remove" data-idx="${idx}">移除</button>
        </div>`;
    }).join('');
    el.querySelectorAll('.btn-queue-remove').forEach((btn) => {
      btn.addEventListener('click', () => {
        const i = Number(btn.dataset.idx);
        state.componentOpsQueue.splice(i, 1);
        renderComponentOpsQueue();
      });
    });
  }

  function parseJSONSafe(raw, fallback) {
    if (!raw) return fallback;
    try {
      return typeof raw === 'string' ? JSON.parse(raw) : raw;
    } catch {
      return fallback;
    }
  }

  function renderAutoTestReportHTML(report) {
    if (!report) return '<div class="empty">暂无自动化测试报告</div>';
    const funcData = parseJSONSafe(report.functional_json, { cases: parseJSONSafe(report.functional_json, []) });
    const funcCases = Array.isArray(funcData) ? funcData : (funcData.cases || []);
    const diffData = parseJSONSafe(report.data_diff_json, {});
    const changeRows = diffData?.data_changes?.rows
      || (diffData.component_diffs || []).flatMap((d) => {
        const comp = d.component || d.kind || 'unknown';
        const rows = [];
        (d.added || []).forEach((n) => rows.push({ component: comp, change_type: 'added', name: n }));
        (d.removed || []).forEach((n) => rows.push({ component: comp, change_type: 'removed', name: n }));
        (d.modified || []).forEach((n) => rows.push({ component: comp, change_type: 'modified', name: n }));
        return rows;
      });
    const summary = diffData?.data_changes?.summary || diffData?.summary || {};
    const verdict = report.ai_verdict || '—';
    const passed = !!report.passed;
    const funcRows = funcCases.map((c) => `
      <tr class="${c.passed ? 'ok' : 'bad'}">
        <td>${escapeHtml(c.name || c.target || '—')}</td>
        <td>${escapeHtml(c.target || '—')}</td>
        <td>${c.passed ? '通过' : '失败'}</td>
        <td><code>${escapeHtml((c.detail || '').slice(0, 160))}</code></td>
      </tr>`).join('');
    const dataRows = (changeRows || []).map((r) => `
      <tr>
        <td>${escapeHtml(r.component || '—')}</td>
        <td>${escapeHtml(r.change_type || '—')}</td>
        <td><code>${escapeHtml(String(r.name || ''))}</code></td>
      </tr>`).join('');
    return `
      <div class="auto-test-verdict ${passed ? 'pass' : 'fail'}">
        <strong>${passed ? '总体通过' : '存在问题'}</strong>
        <span>${escapeHtml(verdict)}</span>
      </div>
      <h4 class="auto-test-section-title">5.1 功能测试</h4>
      <table class="auto-test-table">
        <thead><tr><th>探测项</th><th>目标</th><th>结果</th><th>详情</th></tr></thead>
        <tbody>${funcRows || '<tr><td colspan="4">无数据</td></tr>'}</tbody>
      </table>
      <h4 class="auto-test-section-title">5.2 数据量 / 变更对比</h4>
      <p class="auto-test-summary-line">新增 ${summary.added_count ?? '—'} · 删除 ${summary.removed_count ?? '—'} · 修改 ${summary.modified_count ?? '—'}</p>
      <table class="auto-test-table">
        <thead><tr><th>组件</th><th>变更类型</th><th>对象</th></tr></thead>
        <tbody>${dataRows || '<tr><td colspan="3">未检测到结构化 diff（可能 analyzer 离线或未执行组件 diff-report）</td></tr>'}</tbody>
      </table>`;
  }

  function showAutoTestReport(report) {
    const body = $('autoTestPageReport');
    const summary = $('autoTestPageSummary');
    const last = $('autoTestLastSummary');
    if (!body) return;
    if (!report) {
      if (summary) summary.textContent = '暂无报告';
      if (last) last.textContent = '尚未运行';
      body.innerHTML = '<div class="empty">点击「一键自动化测试」开始，或完成组件批量上线后查看报告。</div>';
      return;
    }
    const triggerLabel = report.trigger === 'manual' ? '手动' : '批量上线后';
    const statusText = `${triggerLabel} · ${report.passed ? '通过' : '未通过'}`;
    if (summary) {
      summary.textContent = `${statusText} · ${(report.ai_verdict || '').slice(0, 100)}`;
    }
    if (last) {
      last.textContent = report.passed
        ? `上次：${statusText} — ${new Date(report.created_at).toLocaleString()}`
        : `上次：${statusText} — 请查看下方报告`;
    }
    body.innerHTML = renderAutoTestReportHTML(report);
  }

  async function runManualAutoTest() {
    if (state.busy) return;
    state.busy = true;
    $('loading')?.classList.add('show');
    const btn = $('btnRunManualAutoTest');
    if (btn) btn.disabled = true;
    try {
      const notes = ($('autoTestNotes')?.value || '').trim();
      const report = await api('/api/components/auto-test/run', {
        method: 'POST',
        body: JSON.stringify({
          slot: 'green',
          include_recent_ops: !!$('autoTestIncludeRecent')?.checked,
          recent_limit: 20,
          notes,
          actor: state.user?.username || 'ops',
        }),
      });
      showAutoTestReport(report);
      toast(report.passed ? '自动化测试通过' : '自动化测试完成（存在问题）', report.passed ? 'success' : 'error');
    } catch (err) {
      toast(err.message || '自动化测试失败', 'error');
    } finally {
      state.busy = false;
      if (btn) btn.disabled = false;
      $('loading')?.classList.remove('show');
    }
  }

  async function loadAutoTestReport() {
    try {
      const report = await api('/api/components/auto-test/latest');
      showAutoTestReport(report);
    } catch {
      showAutoTestReport(null);
    }
  }

  async function execComponentBatch() {
    if (!state.componentOpsQueue.length) {
      toast('队列为空，请先加入组件', 'error');
      return;
    }
    const kinds = state.componentOpsQueue.map((q) => componentKindLabel(q.kind)).join(' → ');
    if (!confirm(`确认逐个执行 ${state.componentOpsQueue.length} 个组件到绿环境？\n\n执行顺序：${kinds}\n\n某项失败会记录并继续下一项，已成功的不回滚。`)) return;
    state.busy = true;
    $('loading')?.classList.add('show');
    const resultEl = $('batchResult');
    try {
      const res = await api('/api/components/batch/apply', {
        method: 'POST',
        body: JSON.stringify({
          items: state.componentOpsQueue,
          actor: state.user?.username || 'ops',
        }),
      });
      resultEl.hidden = false;
      const partial = res.failed || (res.fail_count || 0) > 0;
      resultEl.className = `sql-result ${partial ? 'warn' : 'ok'}`;
      resultEl.textContent = formatBatchResult(res);
      if (res.auto_test) {
        showAutoTestReport(res.auto_test);
        toast('自动化测试报告已更新，可在「自动化测试」页查看');
      }
      toast(partial
        ? `逐个执行完成：${res.success_count || 0} 成功，${res.fail_count || 0} 失败`
        : `全部 ${res.success_count || state.componentOpsQueue.length} 项执行成功`);
      state.componentOpsQueue = [];
      renderComponentOpsQueue();
      await loadComponentOpsHistory();
    } catch (err) {
      const data = err?.data || {};
      resultEl.hidden = false;
      resultEl.className = 'sql-result error';
      resultEl.textContent = formatBatchResult(data.result || data, err.message || String(err), true);
      toast(err.message || '批量执行失败', 'error');
      await loadComponentOpsHistory();
    } finally {
      state.busy = false;
      $('loading')?.classList.remove('show');
    }
  }

  function formatBatchResult(res, errMsg, isError) {
    const lines = [];
    if (isError) {
      lines.push(`✗ 请求失败：${errMsg || ''}`);
    } else {
      const ok = res?.success_count ?? 0;
      const bad = res?.fail_count ?? 0;
      lines.push(`逐个执行：${ok} 成功，${bad} 失败`);
      if (res?.message) lines.push(res.message);
      if (res?.auto_test) {
        lines.push('', '—— 自动化测试 ——');
        lines.push(`功能+数据判定：${res.auto_test.passed ? '通过' : '未通过'}`);
        if (res.auto_test.ai_verdict) lines.push(res.auto_test.ai_verdict);
      }
    }
    (res?.results || []).forEach((r, i) => {
      const mark = r.status === 'success' ? '✓' : '✗';
      lines.push('', `${mark} [${i + 1}] ${r.kind} · ${r.status}`);
      if (r.output) lines.push(r.output.slice(0, 800));
    });
    return lines.join('\n');
  }

  function testCaseToQueueItem(kind) {
    const tc = COMPONENT_TEST_CASES[kind];
    if (!tc) return null;
    if (kind === 'mysql') {
      return { kind, slot: 'green', action: 'apply', payload: tc.sql, label: tc.label, deploy_order: COMPONENT_BATCH_ORDER.mysql };
    }
    if (kind === 'redis') {
      return { kind, slot: 'green', action: 'apply', payload: tc.payload, label: tc.label, deploy_order: COMPONENT_BATCH_ORDER.redis };
    }
    if (kind === 'nacos') {
      return { kind, slot: 'green', action: 'apply', payload: tc.payload, label: tc.label, deploy_order: COMPONENT_BATCH_ORDER.nacos };
    }
    if (kind === 'es') {
      return { kind, slot: 'green', action: 'apply', payload: tc.payload, label: tc.label, deploy_order: COMPONENT_BATCH_ORDER.es };
    }
    if (kind === 'kafka') {
      return {
        kind, slot: 'green', action: 'create-topic', ref: tc.topic, node: tc.partitions,
        label: tc.label, deploy_order: COMPONENT_BATCH_ORDER.kafka,
      };
    }
    return null;
  }

  function fillAllComponentTestCases() {
    Object.keys(COMPONENT_TEST_CASES).forEach((kind) => {
      const tc = COMPONENT_TEST_CASES[kind];
      if (kind === 'mysql') {
        $('sqlLabel').value = tc.label;
        $('customSql').value = tc.sql;
      } else if (kind === 'redis') {
        $('redisLabel').value = tc.label;
        $('redisPayload').value = tc.payload;
      } else if (kind === 'nacos') {
        $('nacosLabel').value = tc.label;
        $('nacosRef').value = '';
        $('nacosPayload').value = tc.payload;
      } else if (kind === 'es') {
        $('esLabel').value = tc.label;
        $('esRef').value = '';
        $('esPayload').value = tc.payload;
      } else if (kind === 'kafka') {
        $('kafkaLabel').value = tc.label;
        $('kafkaTopic').value = tc.topic;
        $('kafkaPartitions').value = tc.partitions;
      }
    });
    state.componentOpsQueue = Object.keys(COMPONENT_TEST_CASES).map((kind) => testCaseToQueueItem(kind)).filter(Boolean);
    renderComponentOpsQueue();
    toast('已填入全部测试用例并加入队列（MySQL → Nacos → Redis → ES → Kafka），可一键执行');
  }

  function renderComponentOpsTab(tab) {
    if (tab) state.componentOpsTab = tab;
    const active = state.componentOpsTab || 'mysql';
    document.querySelectorAll('[data-ops-tab]').forEach((btn) => {
      btn.classList.toggle('active', btn.dataset.opsTab === active);
    });
    document.querySelectorAll('[data-ops-panel]').forEach((panel) => {
      const show = panel.dataset.opsPanel === active;
      panel.hidden = !show;
    });
  }

  function showComponentResult(elId, data, isError) {
    const el = $(elId);
    if (!el) return;
    el.hidden = false;
    el.className = `sql-result ${isError ? 'error' : 'ok'}`;
    el.textContent = typeof data === 'string' ? data : JSON.stringify(data, null, 2);
  }

  async function applyComponent(kind, body, resultId) {
    state.busy = true;
    $('loading')?.classList.add('show');
    try {
      const res = await api(`/api/components/${kind}/apply`, {
        method: 'POST',
        body: JSON.stringify({ ...body, kind, slot: 'green', actor: state.user?.username || 'ops' }),
      });
      showComponentResult(resultId, res.output || res, false);
      toast(`${kind.toUpperCase()} 已执行到绿环境`);
      await loadComponentOpsHistory();
      return res;
    } catch (err) {
      showComponentResult(resultId, err.message || String(err), true);
      throw err;
    } finally {
      state.busy = false;
      $('loading')?.classList.remove('show');
    }
  }

  async function rollbackComponent(kind, resultId) {
    if (!confirm(`确认回滚绿环境最近一次 ${kind.toUpperCase()} 操作？`)) return;
    state.busy = true;
    $('loading')?.classList.add('show');
    try {
      const res = await api(`/api/components/${kind}/rollback`, {
        method: 'POST',
        body: JSON.stringify({ slot: 'green', actor: state.user?.username || 'ops' }),
      });
      showComponentResult(resultId, res.output || res, false);
      toast(`${kind.toUpperCase()} 回滚完成`);
      await loadComponentOpsHistory();
    } catch (err) {
      showComponentResult(resultId, err.message || String(err), true);
      toast(err.message || String(err), 'error');
    } finally {
      state.busy = false;
      $('loading')?.classList.remove('show');
    }
  }

  async function loadComponentOpsHistory() {
    const el = $('componentOpsHistory');
    if (!el) return;
    try {
      const list = await api('/api/components/all/history?slot=green');
      if (!list.length) {
        el.innerHTML = '<div class="empty">暂无记录</div>';
        return;
      }
      el.innerHTML = list.map((op) => `
        <div class="report-card">
          <strong>${escapeHtml(op.kind)} · ${escapeHtml(op.status)}</strong>
          <span>${new Date(op.created_at).toLocaleString()} · ${escapeHtml(op.actor)} · ${escapeHtml(op.action)}</span>
          <span>${escapeHtml((op.output || '').slice(0, 200))}</span>
        </div>`).join('');
    } catch (err) {
      el.innerHTML = `<div class="empty">加载失败：${escapeHtml(err.message || String(err))}</div>`;
    }
  }

  async function execRedisOps() {
    const payload = $('redisPayload')?.value.trim();
    if (!payload) throw new Error('请填写 Redis 命令');
    await applyComponent('redis', { payload, label: warValue('redisLabel'), action: 'apply' }, 'redisResult');
  }

  async function execNacosOps() {
    const ref = warValue('nacosRef');
    const payload = $('nacosPayload')?.value.trim();
    if (!ref && !payload) throw new Error('请填写脚本路径或脚本内容');
    await applyComponent('nacos', { ref, payload, label: warValue('nacosLabel'), action: 'apply' }, 'nacosResult');
  }

  async function execEsOps() {
    const ref = warValue('esRef');
    const payload = $('esPayload')?.value.trim();
    if (!ref && !payload) throw new Error('请填写脚本路径或脚本内容');
    await applyComponent('es', { ref, payload, label: warValue('esLabel'), action: 'apply' }, 'esResult');
  }

  async function execKafkaOps() {
    const topic = warValue('kafkaTopic');
    if (!topic) throw new Error('请填写 Topic 名称');
    const partitions = warValue('kafkaPartitions') || '3';
    await applyComponent('kafka', {
      ref: topic,
      node: partitions,
      label: warValue('kafkaLabel'),
      action: 'create-topic',
    }, 'kafkaResult');
  }

  async function loadAdminUsers() {
    if (!isAdmin()) {
      toast('仅管理员可访问', 'error');
      navigate('deploy');
      return;
    }
    try {
      state.adminUsers = await api('/api/admin/users');
      renderAdminUsers();
    } catch (err) {
      $('userList').innerHTML = `<div class="empty">${escapeHtml(err.message || '加载失败')}</div>`;
    }
  }

  function renderAdminUsers() {
    const el = $('userList');
    if (!el) return;
    if (!state.adminUsers.length) {
      el.innerHTML = '<div class="empty">暂无用户</div>';
      return;
    }
    el.innerHTML = `
      <table class="war-table">
        <thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>创建时间</th><th>操作</th></tr></thead>
        <tbody>${state.adminUsers.map((u) => `
          <tr data-user="${escapeHtml(u.username)}">
            <td>${escapeHtml(u.username)}</td>
            <td><input class="user-display" value="${escapeHtml(u.display_name || '')}" /></td>
            <td>
              <select class="user-role">
                <option value="normal" ${u.role === 'normal' ? 'selected' : ''}>普通</option>
                <option value="admin" ${u.role === 'admin' ? 'selected' : ''}>管理员</option>
              </select>
            </td>
            <td>${new Date(u.created_at).toLocaleString()}</td>
            <td>
              <button type="button" class="btn btn-ghost btn-save-user">保存</button>
              <button type="button" class="btn btn-ghost btn-reset-user-pass">重置密码</button>
              <button type="button" class="btn btn-ghost mini-danger btn-del-user">删除</button>
            </td>
          </tr>`).join('')}</tbody>
      </table>`;
    el.querySelectorAll('.btn-save-user').forEach((btn) => {
      btn.addEventListener('click', () => saveAdminUser(btn.closest('tr')));
    });
    el.querySelectorAll('.btn-reset-user-pass').forEach((btn) => {
      btn.addEventListener('click', () => resetAdminUserPass(btn.closest('tr')));
    });
    el.querySelectorAll('.btn-del-user').forEach((btn) => {
      btn.addEventListener('click', () => deleteAdminUser(btn.closest('tr')));
    });
  }

  async function saveAdminUser(row) {
    const username = row?.dataset.user;
    if (!username) return;
    const displayName = row.querySelector('.user-display')?.value.trim();
    const role = row.querySelector('.user-role')?.value || 'normal';
    await api(`/api/admin/users/${encodeURIComponent(username)}`, {
      method: 'PATCH',
      body: JSON.stringify({ display_name: displayName, role }),
    });
    toast('用户已更新');
    await loadAdminUsers();
  }

  async function resetAdminUserPass(row) {
    const username = row?.dataset.user;
    const password = prompt(`为 ${username} 设置新密码（至少 8 位）`);
    if (!password) return;
    await api(`/api/admin/users/${encodeURIComponent(username)}`, {
      method: 'PATCH',
      body: JSON.stringify({ password }),
    });
    toast('密码已重置');
  }

  async function deleteAdminUser(row) {
    const username = row?.dataset.user;
    if (!username || !confirm(`确认删除用户 ${username}？`)) return;
    await api(`/api/admin/users/${encodeURIComponent(username)}`, { method: 'DELETE' });
    toast('用户已删除');
    await loadAdminUsers();
  }

  async function createAdminUser() {
    const username = warValue('newUserName');
    const password = $('newUserPass')?.value || '';
    const displayName = warValue('newUserDisplay') || username;
    const role = $('newUserRole')?.value || 'normal';
    if (!username || !password) throw new Error('用户名和密码必填');
    await api('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify({ username, password, display_name: displayName, role }),
    });
    $('newUserName').value = '';
    $('newUserPass').value = '';
    $('newUserDisplay').value = '';
    toast('用户创建成功');
    await loadAdminUsers();
  }

  async function loadBlueTemplateIntoEditor() {
    const id = $('blueSqlTemplate')?.value;
    if (!id) { toast('请先选择模板', 'error'); return; }
    const data = await api(`/api/migrations/${id}`);
    $('blueCustomSql').value = data.sql || '';
    if (!$('blueSqlLabel')?.value.trim()) $('blueSqlLabel').value = id;
    toast('模板已填入蓝库编辑器，请检查后再执行');
  }

  function formatSqlError(msg) {
    const raw = String(msg || '');
    const lines = raw.split('\n').filter((l) => !l.includes('Using a password on the command line'));
    const text = lines.join('\n').trim() || raw;
    if (/1050.*already exists/i.test(text)) {
      const m = text.match(/Table '([^']+)' already exists/i);
      const tbl = m ? m[1] : '该表';
      return `表 ${tbl} 已存在，无需重复建表。\n\n若只是确认结构，可执行：\nSHOW CREATE TABLE ${tbl};\n\n若要改字段，请用 ALTER TABLE，或从模板选「001」使用 CREATE TABLE IF NOT EXISTS。\n\n---\n${text}`;
    }
    if (/1060.*Duplicate column/i.test(text)) {
      return `字段已存在，无需重复 ADD COLUMN。\n\n---\n${text}`;
    }
    if (/1091.*Can't DROP/i.test(text)) {
      return `要删除的字段/索引不存在，请先用 SHOW COLUMNS 确认当前表结构。\n\n---\n${text}`;
    }
    return text;
  }

  function formatSqlResult(res, okText) {
    const lines = [okText];
    if (res?.backup_path) lines.push(`执行前快照：${res.backup_path}`);
    if (res?.restored) lines.push('失败恢复：已使用执行前快照恢复');
    if (res?.output) lines.push('', res.output);
    return lines.join('\n');
  }

  function formatSqlFailure(err) {
    const data = err?.data || {};
    const result = data.result;
    const lines = [`✗ 执行失败`, formatSqlError(data.error || err?.message || String(err))];
    if (result?.backup_path) lines.push('', `执行前快照：${result.backup_path}`);
    if (result?.restored) lines.push('失败恢复：已使用执行前快照恢复');
    if (result?.output) lines.push('', result.output);
    return lines.join('\n');
  }

  async function executeCustomSql() {
    if (state.busy) return;
    const sql = $('customSql').value.trim();
    if (!sql) { toast('请先填写 SQL', 'error'); return; }
    const actor = $('author').value.trim() || 'ops';
    const label = $('sqlLabel').value.trim() || 'custom';
    if (!confirm(`确认将以下 SQL 执行到绿环境 MySQL？\n\n目标：osh-g-mysql / backstage\n\n执行前会生成快照；失败会自动尝试恢复。`)) return;

    state.busy = true;
    $('loading').classList.add('show');
    $('btnExecSql').disabled = true;
    const resultEl = $('sqlResult');
    resultEl.hidden = true;
    try {
      const res = await api('/api/sql/execute', {
        method: 'POST',
        body: JSON.stringify({ sql, actor, label }),
      });
      resultEl.hidden = false;
      resultEl.textContent = formatSqlResult(res, '✓ 执行成功');
      resultEl.className = 'sql-result ok';
      toast('SQL 已执行到绿库');
    } catch (err) {
      resultEl.hidden = false;
      resultEl.textContent = formatSqlFailure(err);
      resultEl.className = 'sql-result err';
      toast(formatSqlError(err.message || String(err)).split('\n')[0], 'error');
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      $('btnExecSql').disabled = false;
    }
  }

  async function loadTraffic() {
    state.trafficLoading = true;
    renderBlueUI();
    renderBlueDeployUI();
    try {
      const t = await api('/api/traffic/status');
      state.traffic = t;
      const active = t.active || 'unknown';
      const label = active === 'green' ? '生产流量：绿' : active === 'blue' ? '生产流量：蓝（绿环境仅预发）' : '生产流量：未知';
      $('footerTraffic').textContent = label;
      renderTrafficUI();
      renderBlueUI();
      renderBlueDeployUI();
    } catch {
      state.traffic = null;
      $('footerTraffic').textContent = '生产流量：检测失败';
      renderBlueUI();
      renderBlueDeployUI();
    } finally {
      state.trafficLoading = false;
      renderBlueUI();
      renderBlueDeployUI();
    }
  }

  function renderTrafficUI() {
    const pill = $('trafficActivePill');
    const desc = $('trafficStatusDesc');
    const raw = $('trafficRaw');
    const btnGreen = $('btnToGreen');
    const btnBlue = $('btnToBlue');
    const lockBanner = $('trafficLockBanner');
    const lockText = $('trafficLockText');
    if (!pill) return;

    const t = state.traffic || {};
    const active = t.active || 'unknown';
    const busy = isAnyDeployBusy();

    pill.textContent = active === 'green' ? '绿环境' : active === 'blue' ? '蓝环境' : '未知';
    pill.className = `traffic-pill ${active}`;

    if (active === 'green') {
      desc.textContent = '当前 :80 入口指向绿环境，用户访问的是绿库与绿应用。';
    } else if (active === 'blue') {
      desc.textContent = '当前 :80 入口指向蓝环境（正常生产）。绿环境 :28080 仅用于预发验收。';
    } else {
      desc.textContent = '无法解析当前流量状态，请展开下方查看脚本输出。';
    }

    if (raw) raw.textContent = t.raw || '—';

    if (lockBanner && lockText) {
      lockBanner.hidden = !busy;
      if (busy) {
        lockText.textContent = `发布单「${state.activeDeploy?.title || ''}」正在部署中，请等待完成后再切流`;
      }
    }

    if (btnGreen) {
      btnGreen.disabled = busy || active === 'green' || state.busy;
      btnGreen.title = active === 'green' ? '已在绿环境' : busy ? '部署进行中' : '';
    }
    if (btnBlue) {
      btnBlue.disabled = busy || active === 'blue' || state.busy;
      btnBlue.title = active === 'blue' ? '已在蓝环境' : busy ? '部署进行中' : '';
    }
    renderBlueUI();
  }

  async function loadTrafficHistory() {
    const el = $('trafficHistory');
    if (!el) return;
    try {
      const list = await api('/api/traffic/history');
      if (!list.length) {
        el.innerHTML = '<div class="empty">暂无切流记录</div>';
        return;
      }
      el.innerHTML = list.map((e) => {
        const time = new Date(e.created_at).toLocaleString();
        const reason = e.reason ? ` · ${escapeHtml(e.reason)}` : '';
        return `
          <div class="traffic-history-item">
            <strong>${escapeHtml(e.actor || 'ops')}</strong>
            <span class="arrow">${escapeHtml(e.from_slot)} → ${escapeHtml(e.to_slot)}</span>
            <span class="meta">${time}${reason}</span>
          </div>`;
      }).join('');
    } catch {
      el.innerHTML = '<div class="empty">加载失败</div>';
    }
  }

  async function loadTrafficPage() {
    await loadTraffic();
    await loadTrafficHistory();
    renderTrafficUI();
  }

  async function switchTraffic(target) {
    if (state.busy) return;
    if (isAnyDeployBusy()) {
      toast('有部署任务进行中，请等待完成后再切流', 'error');
      return;
    }

    const active = state.traffic?.active;
    if (target === 'green' && active === 'green') {
      toast('当前已在绿环境', 'error');
      return;
    }
    if (target === 'blue' && active === 'blue') {
      toast('当前已在蓝环境', 'error');
      return;
    }

    const toLabel = target === 'green' ? '绿环境' : '蓝环境';
    const fromLabel = active === 'green' ? '绿' : active === 'blue' ? '蓝' : '当前';
    const reason = $('trafficReason')?.value.trim() || '';
    const actor = $('author').value.trim() || 'ops';

    const warn = target === 'green'
      ? '确认将生产 :80 切到绿环境？\n\n请确保绿环境已部署并通过验收。'
      : '确认将生产 :80 切回蓝环境？\n\n将执行 to-blue --resume-cron 恢复定时同步。';
    if (!confirm(`${warn}\n\n${fromLabel} → ${toLabel}`)) return;

    state.busy = true;
    $('loading').classList.add('show');
    $('btnToGreen').disabled = true;
    $('btnToBlue').disabled = true;
    try {
      const path = target === 'green' ? '/api/traffic/to-green' : '/api/traffic/to-blue';
      const st = await api(path, {
        method: 'POST',
        body: JSON.stringify({ actor, reason }),
      });
      state.traffic = st;
      renderTrafficUI();
      await loadTrafficHistory();
      const label = st.active === 'green' ? '绿环境' : st.active === 'blue' ? '蓝环境' : '目标环境';
      toast(`切流成功，当前生产：${label}`);
      $('footerTraffic').textContent = st.active === 'green' ? '生产流量：绿' : st.active === 'blue' ? '生产流量：蓝（绿环境仅预发）' : '生产流量：未知';
    } catch (err) {
      toast(err.message || String(err), 'error');
      await loadTraffic();
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      renderTrafficUI();
    }
  }

  async function loadList() {
    const list = filterByTarget(await api('/api/releases'), 'green');
    const el = $('list');
    if (!list.length) {
      el.innerHTML = '<div class="empty">还没有历史记录</div>';
      return;
    }
    el.innerHTML = list.map((r) => `
      <button type="button" class="release-item ${state.current?.id === r.id ? 'active' : ''}" data-id="${r.id}">
        <strong>${escapeHtml(r.title)}</strong>
        <span class="meta">${escapeHtml(r.status)} · ${new Date(r.updated_at).toLocaleString()}</span>
      </button>`).join('');
    el.querySelectorAll('.release-item').forEach((node) => {
      node.addEventListener('click', () => select(node.dataset.id));
    });
  }

  async function loadDeployAutoTestReport(releaseId) {
    const fold = $('autoTestFold');
    const body = $('deployAutoTestReport');
    if (!fold || !body || !releaseId) return;
    try {
      const report = await api(`/api/releases/${releaseId}/test-report`);
      if (!report) {
        fold.hidden = true;
        return;
      }
      fold.hidden = false;
      body.innerHTML = renderAutoTestReportHTML(report);
    } catch {
      fold.hidden = true;
    }
  }

  async function select(id) {
    state.current = await api(`/api/releases/${id}`);
    renderUI();
    renderLogs(state.current);
    await loadList();
    schedulePoll();
    toast('已切换到：' + state.current.title);
  }

  function resetDeploy() {
    if (isAnyDeployBusy()) {
      toast('有部署任务进行中，请等待完成后再新建', 'error');
      return;
    }
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
    state.current = null;
    $('title').value = '';
    $('logFold').open = false;
    const resetBtn = $('btnNewDeploy');
    if (resetBtn) resetBtn.textContent = '再部署一遍 →';
    renderUI();
    renderLogs(null);
    loadList();
    toast('已重置，请填写新的发布名称开始部署');
  }

  async function autoPickRelease(list) {
    const greenList = filterByTarget(list, 'green');
    const active = greenList.find((r) => {
      const step = currentStep(r);
      return step > 0 || isDeployInProgress(r);
    });
    if (active) await select(active.id);
  }

  async function createRelease() {
    const title = $('title').value.trim();
    if (!title) throw new Error('请填写发布名称');
    const author = $('author').value.trim();
    if (!author) throw new Error('请填写你的名字');
    const rel = await api('/api/releases', {
      method: 'POST',
      body: JSON.stringify({
        title,
        commit_sha: 'green-deploy',
        author,
        level: 'normal',
        repo: 'juege-osh/osh',
        items: [{
          title: '前后端绿环境部署',
          type: 'code',
          ref: 'deploy-prod',
          developer: $('developer').value,
          expected_impact: $('impact').value,
          reviewer1: $('rev1').value,
          reviewer2: $('rev2').value,
        }],
      }),
    });
    state.current = rel;
    toast('第 1 步完成！继续点绿色按钮');
  }

  async function submitReview() {
    const actor = $('author').value.trim() || 'ops';
    state.current = await api(`/api/releases/${state.current.id}/submit-review`, {
      method: 'POST',
      body: JSON.stringify({ actor }),
    });
    toast('第 2 步完成！继续点绿色按钮');
  }

  async function completeApproval() {
    for (const item of state.current.items || []) {
      if (!reviewsOK(item)) {
        for (const reviewer of [item.reviewer1, item.reviewer2]) {
          await api(`/api/items/${item.id}/reviews`, {
            method: 'POST',
            body: JSON.stringify({
              reviewer, tested: true,
              demo_seen: reviewer !== item.developer,
              result: 'approve', comment: '通过',
            }),
          });
        }
      }
    }
    state.current = await api(`/api/releases/${state.current.id}`);
    if (isBoss()) {
      if (state.current.level === 'urgent') {
        for (const item of state.current.items || []) {
          if (!item.boss_approved) {
            await api(`/api/items/${item.id}/boss-approve`, {
              method: 'POST',
              body: JSON.stringify({ reviewer: state.user.username, comment: '紧急上线逐项确认' }),
            });
          }
        }
        state.current = await api(`/api/releases/${state.current.id}`);
      }
      state.current = await api(`/api/releases/${state.current.id}/boss-approve`, {
        method: 'POST',
        body: JSON.stringify({ reviewer: state.user.username, comment: '终审通过' }),
      });
      toast('终审通过，可以部署了');
    } else {
      toast('双评审已提交，等待 juege 终审');
    }
  }

  async function bossApproveOnly() {
    if (state.current.level === 'urgent') {
      for (const item of state.current.items || []) {
        if (!item.boss_approved) {
          await api(`/api/items/${item.id}/boss-approve`, {
            method: 'POST',
            body: JSON.stringify({ reviewer: state.user.username, comment: '紧急上线逐项确认' }),
          });
        }
      }
      state.current = await api(`/api/releases/${state.current.id}`);
    }
    state.current = await api(`/api/releases/${state.current.id}/boss-approve`, {
      method: 'POST',
      body: JSON.stringify({ reviewer: state.user.username, comment: '终审通过' }),
    });
    toast('终审通过，可以部署了');
  }

  async function deployGreen() {
    if (isOtherDeployBusy(state.current)) {
      throw new Error(`发布单「${state.activeDeploy.title}」正在部署中，请等待完成`);
    }
    const actor = $('author').value.trim() || 'ops';
    state.current = { ...state.current, status: 'deploying' };
    state.activeDeploy = {
      busy: true,
      id: state.current.id,
      title: state.current.title,
      status: 'deploying',
    };
    renderUI();
    renderDeployLock();

    const rel = await api(`/api/releases/${state.current.id}/deploy`, {
      method: 'POST',
      body: JSON.stringify({ actor }),
    });
    state.current = rel;
    toast('已触发部署，正在等待 GitHub Actions…');
    $('logFold').open = true;
    await loadActiveDeploy();
    schedulePoll();
  }

  async function cancelDeployRelease(release) {
    if (!release?.id) return;
    const actor = $('author').value.trim() || state.user?.username || 'ops';
    if (!confirm('确定终止当前部署？\n\n将取消 GitHub Actions 并把环境回滚到本次部署前的版本。')) return;
    state.busy = true;
    const cancelBtn = $('btnCancelDeploy');
    const cancelBlueBtn = $('btnCancelBlueDeploy');
    if (cancelBtn) cancelBtn.disabled = true;
    if (cancelBlueBtn) cancelBlueBtn.disabled = true;
    try {
      const rel = await api(`/api/releases/${release.id}/cancel-deploy`, {
        method: 'POST',
        body: JSON.stringify({ actor, reason: 'user cancel' }),
      });
      if (release.deploy_target === 'blue') {
        state.currentBlue = rel;
      } else {
        state.current = rel;
      }
      toast('已终止部署，正在回滚到部署前版本…');
      $('logFold').open = true;
      renderUI();
      renderBlueUI();
      renderLogs(rel);
      schedulePoll();
    } finally {
      state.busy = false;
      if (cancelBtn) cancelBtn.disabled = false;
      if (cancelBlueBtn) cancelBlueBtn.disabled = false;
    }
  }

  async function handleMainAction() {
    if (state.busy) return;
    state.busy = true;
    $('loading').classList.add('show');
    $('mainBtn').disabled = true;
    try {
      const step = currentStep(state.current);
      if (step === 1) await createRelease();
      else if (step === 2) await submitReview();
      else if (step === 3) {
        if (allReviewsOK(state.current.items) && isBoss()) await bossApproveOnly();
        else await completeApproval();
      }
      else if (step === 4) {
        $('loading').classList.remove('show');
        await deployGreen();
      }
      renderUI();
      renderLogs(state.current);
      await loadList();
      if (currentStep(state.current) > 1 && currentStep(state.current) < 5) {
        $('logFold').open = true;
      }
    } catch (err) {
      toast(err.message || String(err), 'error');
    } finally {
      state.busy = false;
      $('loading').classList.remove('show');
      $('mainBtn').disabled = false;
    }
  }

  function schedulePoll() {
    if (state.pollTimer) clearInterval(state.pollTimer);
    state.pollTimer = null;
    if (!state.current) return;
    const step = currentStep(state.current);
    if (!isDeployInProgress(state.current) && step !== 4) return;
    state.pollTimer = setInterval(async () => {
      try {
        if (!state.current) return;
        state.current = await api(`/api/releases/${state.current.id}`);
        renderUI();
        renderLogs(state.current);
        await loadList();
        await loadActiveDeploy();
        if (!isDeployInProgress(state.current)) {
          clearInterval(state.pollTimer);
          state.pollTimer = null;
          const step = currentStep(state.current);
          if (step === 0) toast('部署完成！请打开绿环境验收');
          else if (state.current.status === 'failed') toast('部署失败，请查看日志', 'error');
        }
      } catch { /* ignore */ }
    }, 3000);
  }

  async function bootApp() {
    renderUI();
    renderBlueDeployUI();
    renderBlueLogs(null);
    try {
      await loadHealth();
      await loadTraffic();
      await loadActiveDeploy();
      await loadComponentSyncActive();
      await loadSqlTemplates();
      await loadDeploySnapshots();
      const list = await api('/api/releases');
      await loadList();
      await loadBlueList();
      await autoPickRelease(list);
      await autoPickBlueRelease(list);
      if (!state.current) renderUI();
      if (!state.currentBlue) renderBlueDeployUI();
      if (state.current && (isDeployInProgress(state.current) || state.current.status === 'deploying')) {
        await loadActiveDeploy();
        schedulePoll();
      }
      if (state.currentBlue && (isDeployInProgress(state.currentBlue) || state.currentBlue.status === 'deploying')) {
        await loadActiveDeploy();
        scheduleBlueDeployPoll();
      }
    } catch (err) {
      $('modeBadge').textContent = '未连接';
      $('modeBadge').className = 'badge offline';
      toast('无法连接服务，请先运行 go run ./cmd/server', 'error');
    }
  }

  async function boot() {
    // Login handlers first — must work even if sidebar/DOM init fails (e.g. cached old HTML)
    renderAuthMode();
    $('btnLogin')?.addEventListener('click', submitAuth);
    $('btnAuthSwitch')?.addEventListener('click', toggleAuthMode);
    ['loginUser', 'loginPass', 'registerDisplayName', 'registerPass2'].forEach((id) => {
      $(id)?.addEventListener('keydown', (e) => { if (e.key === 'Enter') submitAuth(); });
    });
    $('btnLogout')?.addEventListener('click', logout);

    try {
      initSidebar();
      initPageFromHash();
    } catch (err) {
      console.error('initSidebar/initPageFromHash failed', err);
    }

    $('btnRollbackGreen')?.addEventListener('click', () => rollbackDeploy('green'));
    $('btnRollbackBlue')?.addEventListener('click', () => rollbackDeploy('blue'));
    $('btnCancelDeploy')?.addEventListener('click', () => cancelDeployRelease(state.current));
    $('btnCancelBlueDeploy')?.addEventListener('click', () => cancelDeployRelease(state.currentBlue));
    $('btnAddChangeItem')?.addEventListener('click', addWarItem);
    $('btnCreateWarRelease')?.addEventListener('click', () => {
      createWarRelease().catch((err) => toast(err.message || String(err), 'error'));
    });
    document.querySelectorAll('[data-component-tab]').forEach((btn) => {
      btn.addEventListener('click', () => selectWarComponent(btn.dataset.componentTab));
    });
    renderWarComponentPicker();
    document.querySelectorAll('[data-war-view]').forEach((btn) => {
      btn.addEventListener('click', () => {
        state.warRoom.view = btn.dataset.warView || 'table';
        renderWarRoom();
      });
    });
    $('mainBtn')?.addEventListener('click', handleMainAction);
    $('btnNewDeploy')?.addEventListener('click', resetDeploy);
    $('blueMainBtn')?.addEventListener('click', handleBlueMainAction);
    $('btnNewBlueDeploy')?.addEventListener('click', resetBlueDeploy);
    $('btnExecSql')?.addEventListener('click', executeCustomSql);
    $('btnExecBlueSql')?.addEventListener('click', executeBlueCustomSql);
    $('btnLoadTemplate')?.addEventListener('click', loadTemplateIntoEditor);
    $('btnBlueLoadTemplate')?.addEventListener('click', loadBlueTemplateIntoEditor);
    $('btnToGreen')?.addEventListener('click', () => switchTraffic('green'));
    $('btnToBlue')?.addEventListener('click', () => switchTraffic('blue'));
    $('btnSyncBlue')?.addEventListener('click', syncBlue);
    $('btnSyncGreenAll')?.addEventListener('click', syncGreenAllComponents);
    document.querySelectorAll('[data-ops-tab]').forEach((btn) => {
      btn.addEventListener('click', () => {
        renderComponentOpsTab(btn.dataset.opsTab);
      });
    });
    $('btnFillMysqlTest')?.addEventListener('click', () => fillComponentTestCase('mysql'));
    $('btnFillRedisTest')?.addEventListener('click', () => fillComponentTestCase('redis'));
    $('btnFillNacosTest')?.addEventListener('click', () => fillComponentTestCase('nacos'));
    $('btnFillEsTest')?.addEventListener('click', () => fillComponentTestCase('es'));
    $('btnFillKafkaTest')?.addEventListener('click', () => fillComponentTestCase('kafka'));
    $('btnFillAllComponentTests')?.addEventListener('click', fillAllComponentTestCases);
    $('btnQueueMysql')?.addEventListener('click', () => {
      try { queueComponentItem('mysql'); } catch (e) { toast(e.message, 'error'); }
    });
    $('btnQueueRedis')?.addEventListener('click', () => {
      try { queueComponentItem('redis'); } catch (e) { toast(e.message, 'error'); }
    });
    $('btnQueueNacos')?.addEventListener('click', () => {
      try { queueComponentItem('nacos'); } catch (e) { toast(e.message, 'error'); }
    });
    $('btnQueueEs')?.addEventListener('click', () => {
      try { queueComponentItem('es'); } catch (e) { toast(e.message, 'error'); }
    });
    $('btnQueueKafka')?.addEventListener('click', () => {
      try { queueComponentItem('kafka'); } catch (e) { toast(e.message, 'error'); }
    });
    $('btnClearComponentQueue')?.addEventListener('click', () => {
      state.componentOpsQueue = [];
      renderComponentOpsQueue();
    });
    $('btnExecComponentBatch')?.addEventListener('click', () => {
      execComponentBatch().catch((e) => toast(e.message || String(e), 'error'));
    });
    $('btnRunManualAutoTest')?.addEventListener('click', () => {
      runManualAutoTest().catch((e) => toast(e.message || String(e), 'error'));
    });
    $('btnExecRedis')?.addEventListener('click', () => execRedisOps().catch((e) => toast(e.message, 'error')));
    $('btnExecNacos')?.addEventListener('click', () => execNacosOps().catch((e) => toast(e.message, 'error')));
    $('btnExecEs')?.addEventListener('click', () => execEsOps().catch((e) => toast(e.message, 'error')));
    $('btnExecKafka')?.addEventListener('click', () => execKafkaOps().catch((e) => toast(e.message, 'error')));
    $('btnRollbackMysql')?.addEventListener('click', () => rollbackComponent('mysql', 'sqlResult'));
    $('btnRollbackRedis')?.addEventListener('click', () => rollbackComponent('redis', 'redisResult'));
    $('btnRollbackNacos')?.addEventListener('click', () => rollbackComponent('nacos', 'nacosResult'));
    $('btnRollbackEs')?.addEventListener('click', () => rollbackComponent('es', 'esResult'));
    $('btnRollbackKafka')?.addEventListener('click', () => rollbackComponent('kafka', 'kafkaResult'));
    $('btnCreateUser')?.addEventListener('click', () => createAdminUser().catch((e) => toast(e.message, 'error')));
    if (await restoreSession()) {
      await bootApp();
    }
  }

  boot();
})();

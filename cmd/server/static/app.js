(() => {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const state = { health: null, current: null, pollTimer: null, busy: false };

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

  async function api(path, opts = {}) {
    const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts });
    let data = {};
    try { data = await res.json(); } catch { /* empty */ }
    if (!res.ok) throw new Error(data.error || res.statusText);
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

  function reviewsOK(item) {
    if (!item) return false;
    const need = new Set([item.reviewer1, item.reviewer2].filter(Boolean));
    const ok = new Set();
    for (const rv of item.reviews || []) {
      if (rv.result === 'approve' && rv.tested) ok.add(rv.reviewer);
    }
    return need.size === 2 && [...need].every((r) => ok.has(r));
  }

  function autoTestStep(rel) {
    return (rel?.steps || []).find((s) => s.step_key === 'auto_test');
  }

  function deployStep(rel) {
    return (rel?.steps || []).find((s) => s.step_key === 'deploy_standby');
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

    if (rel.boss_approved && reviewsOK(rel.items?.[0])) return 4;
    if (rel.status === 'draft') return 2;
    if (rel.status === 'reviewing') return 3;
    return 2;
  }

  function renderStepper(step) {
    document.querySelectorAll('.stepper-item').forEach((el) => {
      const n = Number(el.dataset.step);
      el.classList.remove('active', 'done');
      if (step === 0 || (step > 0 && n < step)) el.classList.add('done');
      if (n === step) el.classList.add('active');
      if (step === -1 && n === 4) el.classList.add('active');
    });
  }

  function renderUI() {
    const rel = state.current;
    const step = currentStep(rel);
    const cfg = STEPS[step] || STEPS[4];

    renderStepper(step === 0 ? 5 : step);

    $('actionForm').style.display = cfg.showForm ? 'block' : 'none';
    $('successBox').hidden = step !== 0;
    $('waitingBox').hidden = !rel || !['deploying', 'testing'].includes(rel.status);
    $('mainBtn').hidden = step === 0 || (rel && ['deploying', 'testing'].includes(rel.status));

    if (step === 0) {
      const deploy = deployStep(rel);
      const auto = autoTestStep(rel);
      const testWarn = auto?.status === 'failed' || rel?.status === 'failed';
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
      return;
    }

    if (rel && ['deploying', 'testing'].includes(rel.status)) {
      const deploy = deployStep(rel);
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
        ? 'GitHub Actions 正在构建并上传到 149…（约 2–5 分钟）'
        : '绿环境已就绪，正在跑自动测试…';
      return;
    }

    $('stepBadge').textContent = cfg.badge;
    $('actionTitle').textContent = cfg.title;
    $('actionDesc').textContent = cfg.desc;
    $('mainBtnText').textContent = cfg.btn;
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
    $('modeBadge').textContent = h.mock_mode ? '演示模式' : '已连接';
    $('modeBadge').className = `badge ${h.mock_mode ? 'mock' : 'live'}`;
    $('ghaBadge').textContent = d.gha_enabled ? 'GitHub 部署' : 'GHA 未配置';
    $('ghaBadge').className = `badge ${d.gha_enabled ? 'live' : 'offline'}`;

    if (d.green_url) {
      $('greenLink').href = d.green_url;
      $('footerGreenLink').href = d.green_url;
      $('footerGreenLink').textContent = d.green_url;
    }
    $('footerBackend').textContent = d.backend_ref || '—';
    $('footerFrontend').textContent = d.frontend_ref || '—';
  }

  async function loadTraffic() {
    try {
      const t = await api('/api/traffic/status');
      const isGreen = (t.output || '').includes('active (by :80): green');
      $('footerTraffic').textContent = isGreen ? '生产流量：绿' : '生产流量：蓝（绿环境仅预发）';
    } catch {
      $('footerTraffic').textContent = '';
    }
  }

  async function loadList() {
    const list = await api('/api/releases');
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

  async function select(id) {
    state.current = await api(`/api/releases/${id}`);
    renderUI();
    renderLogs(state.current);
    await loadList();
    schedulePoll();
    toast('已切换到：' + state.current.title);
  }

  async function autoPickRelease(list) {
    const active = list.find((r) => !['done', 'failed', 'rolledback'].includes(r.status));
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
          ref: 'deploy-149',
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
    const item = state.current.items[0];
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
    const boss = state.health?.deploy?.boss_reviewer || '觉哥';
    state.current = await api(`/api/releases/${state.current.id}/boss-approve`, {
      method: 'POST',
      body: JSON.stringify({ reviewer: boss, comment: '终审通过' }),
    });
    toast('第 3 步完成！可以部署了');
  }

  async function deployGreen() {
    const actor = $('author').value.trim() || 'ops';
    state.current = await api(`/api/releases/${state.current.id}/deploy`, {
      method: 'POST',
      body: JSON.stringify({ actor }),
    });
    toast('已触发部署，请等待…');
    schedulePoll();
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
      else if (step === 3) await completeApproval();
      else if (step === 4) await deployGreen();
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
    if (!state.current || !['deploying', 'testing'].includes(state.current.status)) return;
    state.pollTimer = setInterval(async () => {
      try {
        state.current = await api(`/api/releases/${state.current.id}`);
        renderUI();
        renderLogs(state.current);
        await loadList();
        if (!['deploying', 'testing'].includes(state.current.status)) {
          clearInterval(state.pollTimer);
          state.pollTimer = null;
          if (state.current.status === 'done') toast('部署完成！请打开绿环境验收');
          else if (state.current.status === 'failed') toast('部署失败，请查看日志', 'error');
        }
      } catch { /* ignore */ }
    }, 5000);
  }

  async function boot() {
    $('mainBtn').addEventListener('click', handleMainAction);
    renderUI();
    renderLogs(null);
    try {
      await loadHealth();
      await loadTraffic();
      const list = await api('/api/releases');
      await loadList();
      await autoPickRelease(list);
      if (!state.current) renderUI();
    } catch (err) {
      $('modeBadge').textContent = '未连接';
      $('modeBadge').className = 'badge offline';
      toast('无法连接服务，请先运行 go run ./cmd/server', 'error');
    }
  }

  boot();
})();

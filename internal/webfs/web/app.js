// Front-end controller for the fire-sprinkler hydraulic engine. Talks only
// to the JSON API on the same origin; the page covers: create project →
// create system → run hydraulic calculation → run NFPA compliance → view the
// full report. No build step, no framework.
(function(){
  const $ = id => document.getElementById(id);
  const api = (m,p,b) => fetch(p,{method:m,headers:{'Content-Type':'application/json'},body:b?JSON.stringify(b):undefined}).then(r=>r.json().then(j=>({j,code:r.status})));
  let selectedSys = '';

  async function refreshProjects(){
    const {j} = await api('GET','/api/projects');
    $('projectList').innerHTML = (j.projects||[]).map(p=>`<div class="list-item" data-id="${p.id}"><b>${p.name}</b> · ${p.hazard_class} · ${p.id}</div>`).join('');
    $('projectList').querySelectorAll('.list-item').forEach(el=>{
      el.onclick = ()=>{ selectedSys=''; loadSystems(el.dataset.id); };
    });
  }

  async function loadSystems(projectId){
    const {j} = await api('GET','/api/projects/'+projectId);
    $('systemList').innerHTML = (j.systems||[]).map(s=>`<div class="list-item" data-id="${s.id}"><b>${s.name}</b> · ${s.kind} · state=${s.state} · ${s.id}</div>`).join('');
    $('systemList').querySelectorAll('.list-item').forEach(el=>{
      el.onclick = ()=>{ selectedSys = el.dataset.id; $('sysId').value = selectedSys; };
    });
  }

  $('btnProject').onclick = async ()=>{
    const {j,code} = await api('POST','/api/projects',{name:$('prjName').value,hazard_class:$('prjHazard').value,design_date:0});
    if(code>=300){alert((j&&j.error)||'建项目失败');return;}
    $('prjName').value=''; refreshProjects();
  };

  $('btnSystem').onclick = async ()=>{
    const pid = $('projectList').querySelector('.list-item');
    if(!pid){alert('请先建/选项目');return;}
    const projectId = pid.dataset.id;
    const {j,code} = await api('POST','/api/projects/'+projectId+'/systems',{
      kind:$('sysKind').value, name:$('sysName').value, base_elevation_mm:+$('sysBase').value,
      design_density:+$('sysDensity').value, design_area_dm2:+$('sysArea').value
    });
    if(code>=300){alert((j&&j.error)||'建系统失败');return;}
    $('sysName').value=''; loadSystems(projectId);
  };

  $('btnCalc').onclick = async ()=>{
    if(!selectedSys){alert('请先选系统');return;}
    const {j,code} = await api('POST','/api/systems/'+selectedSys+'/calculate');
    $('result').textContent = code>=300 ? ('ERR '+(j.error||code)) : JSON.stringify(j,null,2);
    if(code<300 && j.hydraulic){ renderComplianceQuick(j); }
  };

  $('btnCompliance').onclick = async ()=>{
    if(!selectedSys){alert('请先选系统');return;}
    const {j,code} = await api('POST','/api/systems/'+selectedSys+'/compliance');
    if(code>=300){$('result').textContent='ERR '+(j.error||code);return;}
    renderChecks(j.checks||[]);
  };

  $('btnReport').onclick = async ()=>{
    if(!selectedSys){alert('请先选系统');return;}
    const {j,code} = await api('GET','/api/systems/'+selectedSys+'/full-report');
    $('result').textContent = code>=300 ? ('ERR '+(j.error||code)) : JSON.stringify(j,null,2);
    if(code<300){ renderChecks(j.compliance||[]); }
  };

  function renderComplianceQuick(j){
    // After a calc, optionally show the hydraulic summary.
    const h = j.hydraulic;
    if(!h) return;
    $('complianceView').innerHTML = `<div class="chk pass"><span>基准点总流量</span><code>${h.base_flow_lpm} L/min</code></div>
      <div class="chk pass"><span>基准点所需压力</span><code>${h.base_required_pressure_mbar} mbar</code></div>`;
  }
  function renderChecks(checks){
    $('complianceView').innerHTML = checks.map(c=>
      `<div class="chk ${c.passed?'pass':'fail'}"><span>${c.rule_code}</span><code>${c.detail}</code></div>`).join('');
  }

  refreshProjects();
})();

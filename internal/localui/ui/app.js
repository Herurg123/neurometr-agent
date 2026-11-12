"use strict";
(() => {
  const $ = id => document.getElementById(id);
  let csrf = "";
  const TEXT = {ok:"Успешно",partial:"Частично",failed:"Ошибка",skipped:"Не выполнялось"};
  const text = value => value === null || value === undefined || value === "" ? "Неизвестно" : String(value);
  const status = value => TEXT[value] || text(value);
  const date = value => {
    const ms = Date.parse(value || "");
    return Number.isNaN(ms) ? "Неизвестно" : new Date(ms).toLocaleString("ru-RU");
  };
  const memory = value => {
    if (typeof value !== "number" || !Number.isFinite(value) || value < 0) return "Неизвестно";
    return (value / (1024 ** 3)).toLocaleString("ru-RU",{maximumFractionDigits:2})+" ГиБ";
  };
  function element(tag,css,textValue) {
    const el = document.createElement(tag);
    if(css) el.className = css;
    if(textValue !== undefined) el.textContent = textValue;
    return el;
  }
  function stat(label,value) {
    const c = element("div","stat");
    c.append(element("label","",label),element("div","",value));
    return c;
  }
  function appendList(parent,title,items) {
    parent.append(element("h3","sectionlabel",title));
    const ul=element("ul","clean");
    for(const item of items) ul.append(element("li","",text(item)));
    parent.append(ul);
  }
  async function api(url, options) {
    const res=await fetch(url,{credentials:"same-origin",cache:"no-store",...options});
    if(!res.ok) throw new Error("HTTP "+res.status);
    return res.json();
  }
  async function openReport(id) {
    const d=await api("/api/report/"+encodeURIComponent(id));
    const root=$("report-content");
    root.replaceChildren();
    $("report-date").textContent="Выполнено: "+date(d.started_at);
    const badge=element("span","status "+d.overall_status,status(d.overall_status));
    const wrapper=element("div");
    wrapper.append(badge);
    const h=d.hardware || {}, os=h.os || {}, cpu=h.cpu || {}, ram=h.ram || {};
    const cards=element("div","cards");
    cards.append(
      stat("Операционная система",text(os.family)+" · "+text(os.version)+" · "+text(os.arch)),
      stat("Процессор",text(cpu.model)),
      stat("Ядра / потоки",text(cpu.physical_cores)+" / "+text(cpu.logical_processors)),
      stat("Физическая RAM",memory(ram.physical_total_bytes)),
      stat("Доступная RAM",memory(ram.available_bytes)),
      stat("NPU","Не проверялось")
    );
    wrapper.append(element("h3","sectionlabel","Оборудование"),cards);
    const gpuItems=Array.isArray(h.gpus)?h.gpus:[];
    if(gpuItems.length){
      for(const gpu of gpuItems){
        const m=gpu.memory || {};
        const c=element("div","stat");
        c.append(element("label","",text(gpu.name)));
        c.append(element("div","", "Выделенная: "+memory(m.dedicated_total_bytes)+
          " · Разделяемая (предел): "+memory(m.shared_total_bytes)));
        c.append(element("small","", "Свободная VRAM: Не проверялось. Обнаружение не подтверждает работу GPU backend."));
        wrapper.append(c);
      }
    }else{
      wrapper.append(element("p","", "GPU не обнаружен доступным сборщиком. Это не доказывает отсутствие GPU."));
    }
    const components=Array.isArray(d.components)?d.components:[];
    appendList(wrapper,"Компоненты",components.map(c=>text(c.component)+": "+status(c.status)));
    const errors=Array.isArray(d.errors)?d.errors:[];
    if(errors.length) appendList(wrapper,"Структурированные ошибки",errors.map(e=>text(e.code)+": "+text(e.message)));
    appendList(wrapper,"Ограничения",Array.isArray(d.limitations)?d.limitations:[]);
    wrapper.append(element("p","note","Каталог "+text(d.catalog_ref?.version)+
      " · "+text(d.catalog_ref?.origin)+". Модели не запускались; измеренные TPS и fit отсутствуют."));
    root.append(wrapper);
    $("download").href="/api/export/"+encodeURIComponent(id);
    $("download").classList.remove("hidden");
    $("raw-json").textContent=JSON.stringify(d,null,2);
    $("raw-details").classList.remove("hidden");
  }
  async function loadHistory() {
    const data=await api("/api/history");
    const holder=$("history-items");holder.replaceChildren();
    const items=data.entries || [];
    if(!items.length) holder.append(element("p","empty","Сохранённых проверок пока нет."));
    for(const item of items){
      const btn=element("button","history-item");
      const meta=element("span");
      meta.append(element("strong","",date(item.started_at)),element("small","",item.id));
      btn.append(meta,element("span","status "+item.overall_status,status(item.overall_status)));
      btn.addEventListener("click",()=>openReport(item.id).catch(()=>setActivity("Не удалось открыть отчёт.")));
      holder.append(btn);
    }
    const warnings=[];
    if(data.invalid_count) warnings.push("Повреждённые/неподдерживаемые файлы пропущены: "+data.invalid_count+".");
    if(data.truncated) warnings.push("Отображена не вся история.");
    $("history-note").textContent=warnings.join(" ");
  }
  function setActivity(message){$("activity").textContent=message;}
  async function run() {
    const button=$("run");button.disabled=true;setActivity("Проверка оборудования…");
    try{
      const entry=await api("/api/run",{
        method:"POST",headers:{"Content-Type":"application/json","X-Neurometr-CSRF":csrf},
        body:"{}"
      });
      await loadHistory();
      await openReport(entry.id);
      setActivity("Диагностика сохранена локально.");
    }catch(err){
      setActivity("Проверка не завершилась. Другая проверка выполняется или локальное хранилище недоступно.");
    }finally{button.disabled=false;}
  }
  async function main(){
    try{
      const session=await api("/api/session");csrf=session.csrf;
      if(typeof csrf!=="string"||csrf.length<32)throw new Error("session");
      $("run").disabled=false;
      $("run").addEventListener("click",run);
      $("refresh").addEventListener("click",()=>loadHistory().catch(()=>setActivity("Не удалось обновить историю.")));
      await loadHistory();setActivity("Готово к проверке.");
    }catch(_){setActivity("Локальная сессия недоступна. Обновите страницу.");}
  }
  main();
})();

import { useEffect, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { citrusAPI } from "../../api/client";
import "./citrus.css";

type Config = { referral_days: number; device_stars: number; device_rub: number; grace_inbound: number; grace_days: number; xray_auto: boolean; auto_name: string; admin_ids: number[] };
type GraceOption = { server_id: number; server_name: string; inbound_id: number; inbound_name: string };
type Source = { id: number; name: string; url: string; enabled: boolean; tariff_ids: string; interval_minutes: number; fetched_at: number; error: string };
const empty: Config = { referral_days: 0, device_stars: 0, device_rub: 0, grace_inbound: 0, grace_days: 1, xray_auto: false, auto_name: "Авто", admin_ids: [] };
const list = (s: string) => s.split(/[,\s]+/).filter(Boolean).map(Number);

export function CitrusPage() {
 const qc = useQueryClient();
 const config = useQuery({queryKey:["citrus-config"],queryFn:()=>citrusAPI<Config>("settings")});
 const sources = useQuery({queryKey:["citrus-sources"],queryFn:()=>citrusAPI<Source[]>("sources")});
 const graceOptions = useQuery({queryKey:["citrus-grace-options"],queryFn:()=>citrusAPI<GraceOption[]>("grace-options")});
 const [pickedServer,setPickedServer] = useState<number | null>(null);
 const [draft,setDraft] = useState(empty);
 const [admins,setAdmins] = useState("");
 const [note,setNote] = useState("");
 const [busy,setBusy] = useState(false);
 const [source,setSource] = useState({id:0,name:"",url:"",enabled:true,tariff_ids:"",interval_minutes:60});
 useEffect(()=>{if(config.data){setDraft({...empty,...config.data});setPickedServer(null);setAdmins((config.data.admin_ids??[]).join(", "))}},[config.data]);
 const run = async (f:()=>Promise<unknown>)=>{setBusy(true);setNote("");try{await f();setNote("Сохранено.");await qc.invalidateQueries({queryKey:["citrus-sources"]})}catch(e){setNote(e instanceof Error?e.message:"Ошибка запроса")}finally{setBusy(false)}};
 const num = (key: keyof Config,value: string)=>setDraft({...draft,[key]:Number(value)});
 const save = (e:FormEvent)=>{e.preventDefault();void run(async()=>{await citrusAPI("settings","PUT",{...draft,admin_ids:list(admins)});await qc.invalidateQueries({queryKey:["citrus-config"]})})};
 const selectedServer = pickedServer ?? graceOptions.data?.find(o=>o.inbound_id===draft.grace_inbound)?.server_id ?? 0;
 const servers = [...new Map((graceOptions.data??[]).map(o=>[o.server_id,o.server_name])).entries()];
 const saveSource=(e:FormEvent)=>{e.preventDefault();void run(async()=>{await citrusAPI("sources","PUT",{...source,tariff_ids:list(source.tariff_ids)});setSource({id:0,name:"",url:"",enabled:true,tariff_ids:"",interval_minutes:60})})};
 if(config.isPending)return <p>Загрузка настроек…</p>;
 if(config.error)return <p role="alert">Не удалось загрузить настройки. {config.error.message}</p>;
 return <div className="cg-page">
  <header><p className="cg-eyebrow">CITRUS GATE</p><h1>Возможности сервиса</h1><p>Подписки, приглашения и управление клиентами.</p></header>
  <div className="cg-note" role="status" aria-live="polite">{note}</div>
  <form onSubmit={save}>
   <div className="cg-grid">
    <section className="cg-card"><span className="cg-index">01</span><h2>Реферальная программа</h2><p>Бонус за первую покупку приглашённого друга. Возврат платежа отменяет бонус.</p><label>Бонусных дней<input type="number" min="0" max="365" value={draft.referral_days} onChange={e=>num("referral_days",e.target.value)} required/></label><small>0 — выключено. Личная ссылка доступна через /ref и Mini App.</small></section>
    <section className="cg-card"><span className="cg-index">02</span><h2>Докупка устройств</h2><p>Клиент покупает +1 устройство без изменения срока подписки. Покупка сохраняется при продлении.</p><label>Цена в Stars<input type="number" min="0" max="10000" value={draft.device_stars} onChange={e=>num("device_stars",e.target.value)} required/></label><label>Цена в копейках<input type="number" min="0" max="100000000" value={draft.device_rub} onChange={e=>num("device_rub",e.target.value)} required/></label><small>0 — способ оплаты выключен. Нужна настроенная оплата; команда /devicesplus.</small></section>
    <section className="cg-card"><span className="cg-index">03</span><h2>Доступ для продления</h2><p>После окончания подписки клиент открывает Telegram через выбранный вами сервер. Доступ заканчивается через указанное число дней.</p><label>Дней после окончания подписки<input type="number" min="0" step="1" value={draft.grace_days} onChange={e=>num("grace_days",e.target.value)} required/></label><label>Сервер для продления<select value={selectedServer} onChange={e=>{setPickedServer(Number(e.target.value));setDraft({...draft,grace_inbound:0})}} disabled={graceOptions.isPending||!!graceOptions.error}><option value="0">Не выбран — доступ выключен</option>{servers.map(([id,name])=><option key={id} value={id}>{name} · №{id}</option>)}</select></label><label>Подключение на сервере<select value={draft.grace_inbound} onChange={e=>num("grace_inbound",e.target.value)} disabled={!selectedServer}><option value="0">Выберите подключение</option>{graceOptions.data?.filter(o=>o.server_id===selectedServer).map(o=><option key={o.inbound_id} value={o.inbound_id}>{o.inbound_name} · №{o.inbound_id}</option>)}{draft.grace_inbound>0&&!graceOptions.data?.some(o=>o.inbound_id===draft.grace_inbound)&&<option value={draft.grace_inbound}>Недоступное подключение №{draft.grace_inbound}</option>}</select></label>{graceOptions.error&&<p role="alert">Не удалось загрузить серверы.</p>}{graceOptions.data?.length===0&&<p>Сначала добавьте включённый сервер с персональным подключением.</p>}<small>0 дней или отсутствие подключения выключает функцию. Срок считается от окончания подписки, а не от изменения настройки. Подключение должно быть разрешено клиенту его тарифом. Доступны только адреса Telegram на порту 443; Mini App и сайты оплаты через этот доступ не открываются. Все ноды должны использовать эту сборку.</small></section>
    <section className="cg-card"><span className="cg-index">04</span><h2>Автовыбор сервера</h2><p>Именованная группа Xray: приложение измеряет задержку через серверы и выбирает доступный с наименьшей задержкой.</p><label className="cg-check"><input type="checkbox" checked={draft.xray_auto} onChange={e=>setDraft({...draft,xray_auto:e.target.checked})}/>JSON-подписка для приложений Xray</label><label>Название группы<input maxLength={48} value={draft.auto_name} onChange={e=>setDraft({...draft,auto_name:e.target.value})} required/></label><small>Для клиентов с поддержкой полных JSON-конфигураций Xray и observatory. Обычный формат доступен с ?format=uri; JSON — ?format=xray. Совместимость конкретной версии приложения проверьте перед включением.</small></section>
    <section className="cg-card"><span className="cg-index">05</span><h2>Администратор в Telegram</h2><p>/admin открывает клиентов, тарифы, ноды и устройства. Продление, отключение и отвязка требуют подтверждения в боте.</p><label>Telegram ID администраторов<input value={admins} onChange={e=>setAdmins(e.target.value)} placeholder="123456789, 987654321"/></label><small>Используйте числовые ID. Пустой список выключает административные команды.</small></section>
    <section className="cg-card"><span className="cg-index">06</span><h2>Переезд панели</h2><p>В проект входит перенос базы, настроек, сертификатов и ключей с проверкой архива перед восстановлением.</p><p>На исходном сервере: <code>bash citrus move root@new-server</code></p><small>Старый сервер останавливается перед финальной копией. Ссылки сохраняются при переносе домена и сохранении порта. После переноса проверьте ноды и укажите новый адрес локальной ноды.</small></section>
   </div>
   <button className="cg-primary" disabled={busy}>Сохранить настройки</button>
  </form>
  <section className="cg-card cg-external"><span className="cg-index">07</span><h2>Внешние подписки как серверы</h2><p>HTTPS-подписки в формате Clash YAML или списка ссылок. Серверы получают ваше название и обновляются по расписанию.</p><p>Трафик внешнего провайдера идёт напрямую к его серверам. Лимиты устройств и трафика вашей панели к нему не применяются; учёт ведёт поставщик.</p>
   {sources.error&&<p role="alert">Не удалось загрузить источники.</p>}
   <div className="cg-sources">{sources.data?.map(s=><article key={s.id}><div><strong>{s.name}</strong><p>{s.enabled?"Включена":"Выключена"} · {s.fetched_at?new Date(s.fetched_at*1000).toLocaleString():"Ещё не загружена"}</p>{s.error&&<p role="alert">Последнее обновление: {s.error}</p>}</div><div className="cg-actions"><button disabled={busy} onClick={()=>void run(()=>citrusAPI(`sources/${s.id}/refresh`,"POST"))}>Обновить</button><button disabled={busy} onClick={()=>setSource({...s,tariff_ids:JSON.parse(s.tariff_ids??"[]").join(", ")})}>Изменить</button><button disabled={busy} onClick={()=>{if(window.confirm(`Удалить источник «${s.name}»?`))void run(()=>citrusAPI(`sources/${s.id}`,"DELETE"))}}>Удалить</button></div></article>)}</div>
   <form onSubmit={saveSource} className="cg-source-form"><h3>{source.id?"Изменить источник":"Добавить источник"}</h3><label>Ваше название<input value={source.name} maxLength={64} onChange={e=>setSource({...source,name:e.target.value})} required/></label><label>Ссылка HTTPS<input type="url" value={source.url} maxLength={4096} onChange={e=>setSource({...source,url:e.target.value})} required placeholder="https://provider.example/subscription"/></label><label>ID тарифов через запятую<input value={source.tariff_ids} onChange={e=>setSource({...source,tariff_ids:e.target.value})} placeholder="Пусто — все тарифы"/></label><label>Обновлять каждые N минут<input type="number" min="5" max="10080" value={source.interval_minutes} onChange={e=>setSource({...source,interval_minutes:Number(e.target.value)})} required/></label><label className="cg-check"><input type="checkbox" checked={source.enabled} onChange={e=>setSource({...source,enabled:e.target.checked})}/>Включена</label><button className="cg-primary" disabled={busy}>Сохранить источник</button></form>
  </section>
  <footer>Citrus Gate — изменённая версия на основе <a href="https://github.com/Miroshka000/mikan" target="_blank" rel="noreferrer">Mikan</a>. Copyright © 2026 Miroshka and Mikan. GPL-3.0. Неофициальная сборка.</footer>
 </div>;
}

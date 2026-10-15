import { useEffect, useState } from "react";
import { initData, tgMode, subRoot, postJSON, openOutside } from "./net";
type Data={referral_url:string;referral_days:number;invited:number;rewarded:number;device:{stars:number;rub:number;available:{Addons:string[]}}};
export function CitrusSubscriber({token}:{token:string}){
 const[data,setData]=useState<Data|null>(null);const[note,setNote]=useState("");const[busy,setBusy]=useState(false);
 useEffect(()=>{let live=true;if(!tgMode||!token)return;void postJSON<Data>(subRoot+"/tg/citrus",{init_data:initData,token}).then(r=>{if(live&&r.ok)setData(r.data as Data)}).catch(()=>{});return()=>{live=false}},[token]);
 if(!data)return null;
 const buy=async(provider:string)=>{setBusy(true);setNote("");try{const r=await postJSON<{url:string}>(subRoot+"/tg/device-pay",{init_data:initData,token,provider});if(r.ok&&r.data.url)openOutside(r.data.url);else setNote("Не удалось создать счёт. Обновите страницу и проверьте срок подписки.")}catch{setNote("Нет связи с сервером.")}finally{setBusy(false)}};
 return <section style={{padding:"20px",border:"1px solid #8b92a055",borderRadius:"16px",margin:"20px 0"}}>
 {data.referral_url&&<div><h2>Пригласить друзей</h2><p>+{data.referral_days} дней за первую покупку друга. Приглашено: {data.invited}; покупок: {data.rewarded}.</p><a href={data.referral_url} target="_blank" rel="noreferrer">{data.referral_url}</a></div>}
 {(data.device.stars>0||data.device.rub>0)&&<div><h2>Ещё одно устройство</h2><p>Лимит увеличится на 1. Срок подписки останется прежним.</p>{data.device.stars>0&&<button disabled={busy} onClick={()=>void buy("stars")}>Купить за {data.device.stars} Stars</button>}{data.device.rub>0&&data.device.available.Addons?.map(p=><button key={p} disabled={busy} onClick={()=>void buy("addon:"+p)}>Купить за {(data.device.rub/100).toFixed(2)} ₽ · {p}</button>)}</div>}
 <p role="status">{note}</p>
 </section>;
}

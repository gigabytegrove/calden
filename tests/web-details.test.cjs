"use strict";
const assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path"),test=require("node:test");
const read=p=>fs.readFileSync(path.join(__dirname,"..",p),"utf8");
const js=read("web/app.js"),html=read("web/index.html"),css=read("web/app.css");
test("event clicks open details",()=>{
  for(const part of ["if(found)openEventDetails(found)","if(ev)openEventDetails(ev)","if(event)openEventDetails(event)","function openEventDetails(event"])assert.ok(js.includes(part),part);
});
test("event details actions are real",()=>{
  for(const id of ["event-details-dialog","event-details-close","event-details-fields","event-details-edit","event-details-bill","event-details-notes"])assert.ok(html.includes('id="'+id+'"'),id);
  assert.ok(js.includes("requestEventEdit(selected)"));
  assert.ok(js.includes("openBillPayment(selected)"));
});
test("bill entry and allocations are separately grouped",()=>{
  const keys=["bill-payment-history-section","bill-payment-entry-form","bill-allocation-section","bill-payment-no-balance-action"];
  const positions=keys.map(k=>html.indexOf(k));
  assert.ok(positions.every((p,i)=>p>=0&&(i===0||p>positions[i-1])),String(positions));
  assert.ok(css.includes(".bill-payment-no-balance-action"));
});

package api

import (
 "testing"
 "time"
)

func TestCareReminderRecurrence(t *testing.T) {
 date:=func(s string)time.Time{d,e:=time.Parse("2006-01-02",s);if e!=nil{t.Fatal(e)};return d}
 cases:=[]struct{name,start,day,rule string;due bool}{
  {"school oneoff","2026-10-21","2026-10-21","none",true},
  {"school oneoff other day","2026-10-21","2026-10-22","none",false},
  {"weekly due","2026-10-07","2026-10-21","weekly",true},
  {"weekly offday","2026-10-07","2026-10-20","weekly",false},
  {"monthly 31st February","2026-01-31","2026-02-28","monthly",true},
  {"monthly 31st March","2026-01-31","2026-03-31","monthly",true},
  {"yearly leap","2024-02-29","2025-02-28","yearly",true},
  {"before start","2026-10-21","2026-10-20","daily",false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   got:=careOccurrenceValid(date(tc.start),date(tc.day),tc.rule)
   if got!=tc.due{t.Fatalf("due=%v, expected %v",got,tc.due)}
  })
 }
}

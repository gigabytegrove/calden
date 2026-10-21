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

// Advance reminders are anchored to the target occurrence, not the day the
// worker runs. Both one-off and recurring schedules follow the same recurrence.
func TestCareAdvanceLeadDates(t *testing.T) {
 date:=func(s string)time.Time{d,e:=time.Parse("2006-01-02",s);if e!=nil{t.Fatal(e)};return d}
 cases:=[]struct{name,start,today,frequency,target string;lead int;valid bool}{
  {"oneoff seven days", "2026-11-07","2026-10-31","none","2026-11-07",7,true},
  {"no advance", "2026-11-07","2026-10-31","none","",0,false},
  {"oneoff wrong date", "2026-11-07","2026-11-01","none","2026-11-08",7,false},
  {"weekly", "2026-10-07","2026-10-14","weekly","2026-10-21",7,true},
  {"weekly off day", "2026-10-07","2026-10-15","weekly","2026-10-22",7,false},
  {"clamped month", "2026-01-31","2026-02-21","monthly","2026-02-28",7,true},
  {"yearly leap", "2024-02-29","2025-02-21","yearly","2025-02-28",7,true},
  {"invalid 4 days", "2026-11-07","2026-11-03","none","",4,false},
  {"before schedule starts", "2026-12-01","2026-10-21","none","2026-10-28",7,false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  due,ok:=careAdvanceOccurrence(date(tc.start),date(tc.today),tc.frequency,tc.lead)
  if ok!=tc.valid{t.Fatalf("got %v want %v",ok,tc.valid)}
  if tc.valid&&due.Format("2006-01-02")!=tc.target{t.Fatalf("got %s want %s",due.Format("2006-01-02"),tc.target)}
 })}
}

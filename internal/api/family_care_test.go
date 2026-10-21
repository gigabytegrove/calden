package api

import (
 "testing"
 "time"

 "github.com/google/uuid"
)
func careDate(t *testing.T,value string)time.Time{
 t.Helper()
 out,err:=time.Parse("2006-01-02",value);if err!=nil{t.Fatal(err)}
 return out
}
func TestCareOccurrenceRecurrence(t *testing.T){
 rows:=[]struct{name,first,current,rule string;want bool}{
  {"once due","2026-10-21","2026-10-21","none",true},
  {"once next day","2026-10-21","2026-10-22","none",false},
  {"weekly","2026-10-21","2026-11-04","weekly",true},
  {"weekly off","2026-10-21","2026-11-05","weekly",false},
  {"daily","2026-10-21","2026-10-23","daily",true},
  {"before start","2026-10-21","2026-10-20","daily",false},
  {"monthly Jan 31 Feb 28","2026-01-31","2026-02-28","monthly",true},
  {"monthly Jan 31 Feb 27","2026-01-31","2026-02-27","monthly",false},
  {"monthly restores day","2026-01-31","2026-03-31","monthly",true},
  {"yearly leap clamped","2024-02-29","2025-02-28","yearly",true},
  {"yearly leap restored","2024-02-29","2028-02-29","yearly",true},
  {"yearly off","2024-02-29","2025-03-01","yearly",false},
 }
 for _,tc:=range rows{
  t.Run(tc.name,func(t *testing.T){
   got:=careOccurrenceValid(careDate(t,tc.first),careDate(t,tc.current),tc.rule)
   if got!=tc.want{t.Fatalf("got %v want %v",got,tc.want)}
  })
 }
}
func TestCareNextOccurrence(t *testing.T){
 cases:=[]struct{name,first,today,rule,want string}{
  {"overdue one-off","2026-10-11","2026-10-21","none","2026-10-11"},
  {"future one-off","2026-10-31","2026-10-21","none","2026-10-31"},
  {"monthly 31st","2026-01-31","2026-02-01","monthly","2026-02-28"},
  {"annual birthday","2024-06-05","2026-06-06","yearly","2027-06-05"},
  {"weekly next","2026-10-20","2026-10-21","weekly","2026-10-27"},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  got:=careNextOccurrence(careDate(t,tc.first),careDate(t,tc.today),tc.rule)
  if got!=tc.want{t.Fatalf("got %s want %s",got,tc.want)}
 })}
}
func TestCareSubjectValidation(t *testing.T){
 id:=uuid.New()
 cases:=[]struct{name,kind string;input careSubjectInput;valid bool}{
  {"pet","pets",careSubjectInput{Name:" Hazel "},true},
  {"equipment","maintenance",careSubjectInput{Name:"HVAC",Details:"Filter size 16x20"},true},
  {"linked child","school",careSubjectInput{Name:"School record",ChildUserID:&id,SchoolName:"Valdosta"},true},
  {"missing name","pets",careSubjectInput{Name:" "},false},
  {"pet child link","pets",careSubjectInput{Name:"Hazel",ChildUserID:&id},false},
  {"equipment school detail","maintenance",careSubjectInput{Name:"HVAC",SchoolName:"No"},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,ok:=cleanCareSubject(tc.input,tc.kind);if ok!=tc.valid{t.Fatalf("valid %v want %v",ok,tc.valid)}
 })}
}
func TestCareItemKindAndDateValidation(t *testing.T){
 cases:=[]struct{name,kind string;input careItemInput;valid bool}{
  {"deadline","school",careItemInput{Kind:"deadline",Title:"Book report",StartsOn:"2026-10-25"},true},
  {"service","maintenance",careItemInput{Kind:"service",Title:"Change filter",StartsOn:"2026-10-30",RepeatRule:"monthly"},true},
  {"vet","pets",careItemInput{Kind:"appointment",Title:"Annual checkup",StartsOn:"2026-11-01",RepeatRule:"yearly"},true},
  {"bad school medication","school",careItemInput{Kind:"medication",Title:"No",StartsOn:"2026-10-25"},false},
  {"bad pet service","pets",careItemInput{Kind:"service",Title:"No",StartsOn:"2026-10-25"},false},
  {"bad date","pets",careItemInput{Kind:"appointment",Title:"Vet",StartsOn:"2026-02-31"},false},
  {"bad repeat","maintenance",careItemInput{Kind:"service",Title:"Check",StartsOn:"2026-10-25",RepeatRule:"hourly"},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,ok:=cleanCareItem(tc.input,tc.kind);if ok!=tc.valid{t.Fatalf("valid=%v want %v",ok,tc.valid)}
 })}
}

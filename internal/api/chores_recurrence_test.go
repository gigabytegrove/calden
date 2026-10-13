package api

import (
    "testing"
    "time"

    "github.com/google/uuid"
)

func mustChoreDate(t *testing.T,s string) time.Time{
    t.Helper()
    date,err:=choreDate(s)
    if err!=nil{t.Fatal(err)}
    return date
}
func TestChoreRecurrence(t *testing.T){
    tests:=[]struct{rule,start,day string;expected bool}{
        {"none","2026-10-13","2026-10-13",true},
        {"none","2026-10-13","2026-10-14",false},
        {"daily","2026-10-13","2026-10-14",true},
        {"weekly","2026-10-13","2026-10-20",true},
        {"weekly","2026-10-13","2026-10-19",false},
        {"biweekly","2026-10-13","2026-10-27",true},
        {"biweekly","2026-10-13","2026-10-20",false},
        {"monthly","2027-01-31","2027-02-28",true},
        {"monthly","2027-01-31","2027-03-31",true},
        {"monthly","2027-01-31","2027-03-30",false},
        {"yearly","2024-02-29","2025-02-28",true},
        {"yearly","2024-02-29","2028-02-29",true},
        {"daily","2026-10-13","2026-10-12",false},
    }
    for _,test:=range tests{
        t.Run(test.rule+"/"+test.start+"/"+test.day,func(t *testing.T){
            got:=choreDueOn(mustChoreDate(t,test.start),mustChoreDate(t,test.day),test.rule)
            if got!=test.expected{t.Errorf("got %v, expected %v",got,test.expected)}
        })
    }
}
func TestChoreValidation(t *testing.T){
    id:=uuid.New().String()
    base:=choreInput{Title:" Test ",DueDate:"2026-10-13",AssigneeIDs:[]string{id},RecurrenceRule:"monthly"}
    got,people,_,_,errorText:=validateChore(base)
    if errorText!="" || len(people)!=1 || got.Title!="Test"{t.Fatalf("unexpected validation: %s, %#v",errorText,got)}
    duplicate:=base;duplicate.AssigneeIDs=[]string{id,id}
    if _,_,_,_,err:=validateChore(duplicate);err==""{t.Fatal("duplicate participants allowed")}
    until:=base;until.RecurrenceUntil="2026-10-12"
    if _,_,_,_,err:=validateChore(until);err==""{t.Fatal("end before start allowed")}
    invalid:=base;invalid.RecurrenceRule="hourly"
    if _,_,_,_,err:=validateChore(invalid);err==""{t.Fatal("unsupported recurrence allowed")}
    legacy:=choreInput{Title:"Legacy",DueDate:"2026-10-13",AssignedTo:id}
    if _,people,_,_,err:=validateChore(legacy);err!=""||len(people)!=1{t.Fatal("legacy assignee rejected")}
}

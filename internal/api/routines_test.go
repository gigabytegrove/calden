package api

import (
 "testing"
 "time"
 "github.com/google/uuid"
)

func TestRoutineValidation(t *testing.T) {
 person:=uuid.New()
 cases:=[]struct{label string;input routineInput;valid bool}{
  {"valid",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"morning",DaysOfWeek:[]int16{1,2,3},Steps:[]routineStepInput{{Title:"Brush teeth"},{Title:"Get dressed"}}},true},
  {"missing person",routineInput{Title:"Morning",TimeOfDay:"morning",DaysOfWeek:[]int16{1},Steps:[]routineStepInput{{Title:"Brush teeth"}}},false},
  {"zero steps",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"morning",DaysOfWeek:[]int16{1}},false},
  {"invalid day",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"morning",DaysOfWeek:[]int16{8},Steps:[]routineStepInput{{Title:"Step"}}},false},
  {"duplicate day",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"morning",DaysOfWeek:[]int16{2,2},Steps:[]routineStepInput{{Title:"Step"}}},false},
  {"invalid slot",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"midnight",DaysOfWeek:[]int16{1},Steps:[]routineStepInput{{Title:"Step"}}},false},
  {"missing step",routineInput{Title:"Morning",AssignedTo:person,TimeOfDay:"morning",DaysOfWeek:[]int16{1},Steps:[]routineStepInput{{Title:" "}}},false},
 }
 for _,tc:=range cases {
  t.Run(tc.label,func(t *testing.T){
   _,ok:=normalizeRoutine(tc.input)
   if ok!=tc.valid {t.Fatalf("valid=%v want=%v",ok,tc.valid)}
  })
 }
}
func TestRoutineOccurrenceDays(t *testing.T){
 days:=[]int16{1,3,5}
 monday,_:=time.Parse("2006-01-02","2026-10-19")
 if !routineDue(days,monday) {t.Fatal("Monday should recur")}
 if routineDue(days,monday.AddDate(0,0,1)){t.Fatal("Tuesday should not recur")}
 if !routineDue(days,monday.AddDate(0,0,2)){t.Fatal("Wednesday should recur")}
 if !routineDue([]int16{7},monday.AddDate(0,0,6)){t.Fatal("Sunday should recur")}
}
func TestRoutineStableStepIDs(t *testing.T){
 input:=routineInput{Title:"Bedtime",AssignedTo:uuid.New(),DaysOfWeek:[]int16{1,2,3,4,5,6,7},TimeOfDay:"evening",Steps:[]routineStepInput{{Title:"Wash up"},{Title:"Read"}}}
 clean,ok:=normalizeRoutine(input)
 if !ok{t.Fatal("valid routine rejected")}
 steps,err:=allocateRoutineSteps(clean.Steps,nil)
 if err!=nil||len(steps)!=2||steps[0].ID==steps[1].ID{t.Fatalf("new steps not unique: %v",err)}
 renamed:=[]routineStepInput{{ID:steps[0].ID.String(),Title:"Brush teeth"},{ID:steps[1].ID.String(),Title:"Read"}}
 next,err:=allocateRoutineSteps(renamed,steps)
 if err!=nil||next[0].ID!=steps[0].ID{t.Fatalf("step identity not preserved: %v",err)}
 unknown:=[]routineStepInput{{ID:uuid.NewString(),Title:"Fake"}}
 if _,err:=allocateRoutineSteps(unknown,steps);err==nil{t.Fatal("foreign step id accepted")}
 duplicate:=[]routineStepInput{{ID:steps[0].ID.String(),Title:"First"},{ID:steps[0].ID.String(),Title:"Second"}}
 if _,err:=allocateRoutineSteps(duplicate,steps);err==nil{t.Fatal("duplicate step id accepted")}
}

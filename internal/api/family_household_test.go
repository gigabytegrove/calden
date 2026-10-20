package api

import "testing"

func TestFamilyItemValidation(t *testing.T) {
 cases:=[]struct{name string;input familyItemInput;want bool}{
  {"shopping",familyItemInput{Title:"Milk",Quantity:2},true},
  {"recurring",familyItemInput{Title:"Replace filter",DueOn:"2026-10-20",Recurrence:"monthly"},true},
  {"missing title",familyItemInput{Title:"  "},false},
  {"invalid rule",familyItemInput{Title:"Water",Recurrence:"every hour"},false},
  {"recurring without date",familyItemInput{Title:"Water",Recurrence:"daily"},false},
  {"invalid date",familyItemInput{Title:"School",DueOn:"2026-02-30"},false},
  {"large quantity",familyItemInput{Title:"Food",Quantity:1000001},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,err:=validateFamilyItem(tc.input);if (err==nil)!=tc.want{t.Fatalf("validation error=%v; want valid=%v",err,tc.want)}
 })}
}
func TestFamilyModuleRegistry(t *testing.T) {
 for _,key:=range []string{"shopping","school","maintenance","pets","routines"} {
  if _,ok:=familySections[key];!ok{t.Fatalf("missing module: %s",key)}
 }
}

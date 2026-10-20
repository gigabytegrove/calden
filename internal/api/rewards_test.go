package api

import (
 "testing"
 "time"

 "github.com/google/uuid"
)

func TestRewardCatalogValidation(t *testing.T){
 cases:=[]struct{name string;in rewardCatalogInput;valid bool}{
  {"valid",rewardCatalogInput{Title:" Movie night ",CostPoints:25,Active:true},true},
  {"missing title",rewardCatalogInput{CostPoints:20},false},
  {"zero cost",rewardCatalogInput{Title:"Movie",CostPoints:0},false},
  {"negative cost",rewardCatalogInput{Title:"Movie",CostPoints:-1},false},
  {"limit",rewardCatalogInput{Title:"Movie",CostPoints:100000},true},
  {"over limit",rewardCatalogInput{Title:"Movie",CostPoints:100001},false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   out,valid:=validRewardCatalog(tc.in)
   if valid!=tc.valid{t.Fatalf("valid=%v want %v",valid,tc.valid)}
   if tc.name=="valid"&&out.Title!="Movie night"{t.Fatalf("name not normalized: %q",out.Title)}
  })
 }
}
func TestAllowancePlanValidation(t *testing.T){
 who:=uuid.New()
 cases:=[]struct{name string;in allowancePlanInput;valid bool}{
  {"weekly",allowancePlanInput{UserID:who,AmountCents:500,Frequency:"weekly",FirstDue:"2026-10-20",Active:true},true},
  {"monthly",allowancePlanInput{UserID:who,AmountCents:700,Frequency:"monthly",FirstDue:"2026-10-31",Active:true},true},
  {"no user",allowancePlanInput{AmountCents:500,Frequency:"weekly",FirstDue:"2026-10-20"},false},
  {"no amount",allowancePlanInput{UserID:who,Frequency:"weekly",FirstDue:"2026-10-20"},false},
  {"too much",allowancePlanInput{UserID:who,AmountCents:100000001,Frequency:"weekly",FirstDue:"2026-10-20"},false},
  {"bad frequency",allowancePlanInput{UserID:who,AmountCents:500,Frequency:"daily",FirstDue:"2026-10-20"},false},
  {"bad date",allowancePlanInput{UserID:who,AmountCents:500,Frequency:"weekly",FirstDue:"2026-02-30"},false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   if got:=validAllowancePlan(tc.in);got!=tc.valid{t.Fatalf("valid=%v want %v",got,tc.valid)}
  })
 }
}
func TestAllowanceAdvance(t *testing.T){
 parse:=func(s string)time.Time{v,err:=time.Parse("2006-01-02",s);if err!=nil{t.Fatal(err)};return v}
 cases:=[]struct{start,freq string;anchor int;want string}{
  {"2026-10-20","weekly",20,"2026-10-27"},
  {"2026-01-31","monthly",31,"2026-02-28"},
  {"2026-02-28","monthly",31,"2026-03-31"},
  {"2024-01-31","monthly",31,"2024-02-29"},
  {"2024-02-29","monthly",31,"2024-03-31"},
  {"2026-11-30","monthly",30,"2026-12-30"},
  {"2026-12-31","monthly",31,"2027-01-31"},
 }
 for _,tc:=range cases{
  t.Run(tc.start+tc.freq,func(t *testing.T){
   got:=allowanceAdvance(parse(tc.start),tc.freq,tc.anchor).Format("2006-01-02")
   if got!=tc.want{t.Fatalf("got %s want %s",got,tc.want)}
  })
 }
}

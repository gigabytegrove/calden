package api

import (
 "testing"
 "math"
 "net/http/httptest"
 "strings"
)

func TestShoppingListValidation(t *testing.T) {
  cases:=[]struct{name string;input shoppingListInput;valid bool}{
    {"valid",shoppingListInput{Name:" Food ",Shared:true},true},
    {"empty",shoppingListInput{Name:" "},false},
    {"too long",shoppingListInput{Name:string(make([]byte,121))},false},
  }
  for _,tc:=range cases {
    t.Run(tc.name,func(t *testing.T){out,ok:=normalizeShoppingList(tc.input);if ok!=tc.valid{t.Fatalf("got %v want %v",ok,tc.valid)}
      if tc.name=="valid"&&out.Name!="Food"{t.Fatalf("unexpected trim: %q",out.Name)}
    })
  }
}
func TestShoppingItemValidation(t *testing.T) {
  cases:=[]struct{name string;in shoppingItemInput;valid bool}{
    {"valid",shoppingItemInput{Name:" Milk ",Quantity:2},true},
    {"zero",shoppingItemInput{Name:"Milk",Quantity:0},false},
    {"negative",shoppingItemInput{Name:"Milk",Quantity:-1},false},
    {"too small to represent",shoppingItemInput{Name:"Milk",Quantity:0.0001},false},
    {"too big",shoppingItemInput{Name:"Milk",Quantity:1000001},false},
    {"invalid unit",shoppingItemInput{Name:"Milk",Quantity:1,Unit:"javascript"},false},
    {"missing name",shoppingItemInput{Name:" ",Quantity:1},false},
  }
  for _,tc:=range cases {t.Run(tc.name,func(t *testing.T) {
    out,ok:=normalizeShoppingItem(tc.in);if ok!=tc.valid{t.Fatalf("valid=%v want=%v",ok,tc.valid)}
    if tc.name=="valid"&&(out.Name!="Milk"||out.Unit!="each"){t.Fatalf("unexpected normalization: %+v",out)}
  })}
}

func TestShoppingSupplyMetadataValidation(t *testing.T){
 good:=shoppingItemInput{Name:" Batteries ",Quantity:4,Unit:"pack",Category:" Utility "}
 if v,ok:=normalizeShoppingItem(good);!ok||v.Category!="Utility"{t.Fatalf("valid metadata rejected: %+v %v",v,ok)}
 for _,tc:=range []shoppingItemInput{
  {Name:"A",Quantity:1,Category:string(make([]byte,81))},
  {Name:"A",Quantity:math.NaN()},
  {Name:"A",Quantity:math.Inf(1)},
  {Name:"A",Quantity:1,StockOnHand:ptrFloat(-1)},
  {Name:"A",Quantity:1,ReorderAt:ptrFloat(0)},
  {Name:"A",Quantity:1,ReorderAt:ptrFloat(0.0001)},
  {Name:"A",Quantity:1,ReorderAt:ptrFloat(math.Inf(1))},
 }{
  if _,ok:=normalizeShoppingItem(tc);ok{t.Fatalf("accepted invalid supply item: %+v",tc)}
 }
}
func ptrFloat(v float64)*float64{return &v}

func TestShoppingMetadataPresenceAndLegacyCompatibility(t *testing.T){
 for _,tc:=range []struct{name,json string;valid bool;category,stock bool}{
  {"legacy",`{"name":"Milk","quantity":1,"unit":"each","purchased":false}`,true,false,false},
  {"explicit values",`{"name":"Milk","quantity":1,"category":"Dairy","stock_on_hand":0}`,true,true,true},
  {"explicit null",`{"name":"Milk","quantity":1,"stock_on_hand":null,"assigned_to":null}`,true,false,true},
  {"unknown field",`{"name":"Milk","quantity":1,"admin_override":true}`,false,false,false},
  {"malformed type",`{"name":"Milk","quantity":"wrong"}`,false,false,false},
  {"array payload",`[]`,false,false,false},
 }{
  t.Run(tc.name,func(t *testing.T){
   req:=httptest.NewRequest("PUT","/shopping",strings.NewReader(tc.json))
   in,fields,err:=decodeShoppingInput(req)
   if (err==nil)!=tc.valid{t.Fatalf("valid=%v error=%v",tc.valid,err)}
   if err!=nil{return}
   if in.Name!="Milk"||fields["category"]!=tc.category||fields["stock_on_hand"]!=tc.stock{
    t.Fatalf("wrong decoded metadata: %+v %+v",in,fields)
   }
  })
 }
}

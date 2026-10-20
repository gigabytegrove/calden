package api

import "testing"

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
    {"too big",shoppingItemInput{Name:"Milk",Quantity:1000001},false},
    {"invalid unit",shoppingItemInput{Name:"Milk",Quantity:1,Unit:"javascript"},false},
    {"missing name",shoppingItemInput{Name:" ",Quantity:1},false},
  }
  for _,tc:=range cases {t.Run(tc.name,func(t *testing.T) {
    out,ok:=normalizeShoppingItem(tc.in);if ok!=tc.valid{t.Fatalf("valid=%v want=%v",ok,tc.valid)}
    if tc.name=="valid"&&(out.Name!="Milk"||out.Unit!="each"){t.Fatalf("unexpected normalization: %+v",out)}
  })}
}

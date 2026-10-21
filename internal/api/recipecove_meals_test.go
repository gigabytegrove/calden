package api

import (
 "testing"
 "net/http"
 "net/http/httptest"
 "strings"
)

func TestRecipeCoveMealValidation(t *testing.T) {
 base:=coveMealInput{Day:"2026-10-21",Meal:"dinner",RecipeID:12,Servings:4,Note:"Family",RequestID:"family-unique-123"}
 cases:=[]struct{name string;mod func(*coveMealInput);ok bool}{
  {"valid",func(_ *coveMealInput){},true},
  {"no date",func(v *coveMealInput){v.Day=""},false},
  {"bad date",func(v *coveMealInput){v.Day="2026-02-30"},false},
  {"bad meal",func(v *coveMealInput){v.Meal="supper"},false},
  {"bad recipe",func(v *coveMealInput){v.RecipeID=0},false},
  {"bad servings",func(v *coveMealInput){v.Servings=0},false},
  {"huge servings",func(v *coveMealInput){v.Servings=101},false},
  {"short request ID",func(v *coveMealInput){v.RequestID="123"},false},
  {"unsafe request ID",func(v *coveMealInput){v.RequestID="../hello-world"},false},
  {"max note",func(v *coveMealInput){v.Note=string(make([]byte,501))},false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   v:=base;tc.mod(&v)
   if got:=validCoveMeal(v,true);got!=tc.ok{t.Fatalf("got %v expected %v",got,tc.ok)}
  })
 }
 edit:=base;edit.RequestID=""
 if !validCoveMeal(edit,false){t.Fatal("editing an existing meal should not require a new request ID")}
}

func TestRecipeCoveProxyKeepsKeyServerSide(t *testing.T){
 var requests int
 provider:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests++
  if got:=r.Header.Get("X-Forknook-API-Key");got!="fnk_local-test-secret"{
   t.Errorf("unexpected server authentication header: %q",got)
  }
  if r.URL.Path!="/api/v2/integrations/calden/recipes"{
   t.Errorf("unexpected target path: %s",r.URL.Path)
  }
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"items":[{"id":3,"title":"Soup"}]}`))
 }))
 defer provider.Close()
 t.Setenv("RECIPECOVE_URL",provider.URL+"/")
 t.Setenv("RECIPECOVE_SYNC_KEY","fnk_local-test-secret")
 t.Setenv("RECIPECOVE_ALLOW_HTTP","true")
 recorder:=httptest.NewRecorder()
 recipeCoveProxy(recorder,httptest.NewRequest("GET","/api/integrations/recipecove/recipes",nil),
  "GET","/recipes",nil)
 if recorder.Code!=200{t.Fatalf("status %d body=%s",recorder.Code,recorder.Body.String())}
 if requests!=1{t.Fatalf("got %d outbound calls",requests)}
 if strings.Contains(recorder.Body.String(),"fnk_local-test-secret"){
  t.Fatal("integration token leaked to client")
 }
}
func TestRecipeCoveProxyNeverFollowsRedirects(t *testing.T){
 var followed int
 destination:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  followed++
 }))
 defer destination.Close()
 redirect:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  http.Redirect(w,r,destination.URL,http.StatusFound)
 }))
 defer redirect.Close()
 t.Setenv("RECIPECOVE_URL",redirect.URL)
 t.Setenv("RECIPECOVE_SYNC_KEY","fnk_local-test-secret")
 t.Setenv("RECIPECOVE_ALLOW_HTTP","true")
 recorder:=httptest.NewRecorder()
 recipeCoveProxy(recorder,httptest.NewRequest("GET","/api/integrations/recipecove/recipes",nil),
  "GET","/recipes",nil)
 if recorder.Code!=502{t.Fatalf("redirect should fail, got %d",recorder.Code)}
 if followed!=0{t.Fatal("redirect leaked request to a different host")}
}

func TestRecipeCoveRejectsOversizedJSONResponse(t *testing.T){
 provider:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  // A valid JSON value in the first 2 MiB must not conceal surplus bytes.
  _,_=w.Write([]byte(`{"ok":true}`+strings.Repeat(" ",2<<20)))
 }))
 defer provider.Close()
 t.Setenv("RECIPECOVE_URL",provider.URL)
 t.Setenv("RECIPECOVE_SYNC_KEY","fnk_local-test-secret")
 t.Setenv("RECIPECOVE_ALLOW_HTTP","true")
 recorder:=httptest.NewRecorder()
 recipeCoveProxy(recorder,httptest.NewRequest("GET","/meals",nil),"GET","/recipes",nil)
 if recorder.Code!=502{t.Fatalf("oversized response accepted: %d",recorder.Code)}
}

func TestRecipeCoveRejectsInvalidJSONAndNoContent(t *testing.T){
 for _,tc:=range []struct{name string;status int;body string}{
  {"malformed JSON",200,`{"items":`},
  {"no content",204,""},
  {"provider error",500,`{"secret":"must not leak"}`},
 }{
  t.Run(tc.name,func(t *testing.T){
   provider:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    w.WriteHeader(tc.status);_,_=w.Write([]byte(tc.body))
   }))
   defer provider.Close()
   t.Setenv("RECIPECOVE_URL",provider.URL)
   t.Setenv("RECIPECOVE_SYNC_KEY","fnk_local-test-secret")
   t.Setenv("RECIPECOVE_ALLOW_HTTP","true")
   recorder:=httptest.NewRecorder()
   recipeCoveProxy(recorder,httptest.NewRequest("GET","/meals",nil),"GET","/recipes",nil)
   if recorder.Code!=502{t.Fatalf("unexpected status: %d, body=%s",recorder.Code,recorder.Body.String())}
   if strings.Contains(recorder.Body.String(),"must not leak"){t.Fatal("provider error leaked to browser")}
  })
 }
}

func TestRecipeCoveRejectsUnsafeConfiguration(t *testing.T){
 t.Setenv("RECIPECOVE_SYNC_KEY","fnk_local-test-secret")
 t.Setenv("RECIPECOVE_ALLOW_HTTP","false")
 for _,origin:=range []string{
  "http://example.com","https://user:pass@example.com","https://example.com/path",
  "https://example.com?token=secret","ftp://example.com",
 }{
  t.Setenv("RECIPECOVE_URL",origin)
  if _,_,err:=recipeCoveConfig();err==nil{t.Errorf("accepted invalid origin %q",origin)}
 }
}

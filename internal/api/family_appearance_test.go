package api

import "testing"

func TestNormalizeFamilyAppearance(t *testing.T){
 full:=func()familyAppearancePayload{
  p:=familyAppearancePayload{Theme:"forest",Modules:[]familyPresentation{}}
  for _,base:=range approvedPresentationModules{p.Modules=append(p.Modules,base)}
  return p
 }
 valid:=full()
 normalized,ok:=normalizeFamilyAppearance(valid)
 if !ok||len(normalized.Modules)!=len(approvedPresentationModules){t.Fatal("valid family appearance rejected")}
 invalidTheme:=full();invalidTheme.Theme="javascript"
 if _,ok:=normalizeFamilyAppearance(invalidTheme);ok{t.Fatal("unknown theme accepted")}
 missing:=full();missing.Modules=missing.Modules[:len(missing.Modules)-1]
 if _,ok:=normalizeFamilyAppearance(missing);ok{t.Fatal("partial module presentation accepted")}
 duplicate:=full();duplicate.Modules[0].ModuleID=duplicate.Modules[1].ModuleID
 if _,ok:=normalizeFamilyAppearance(duplicate);ok{t.Fatal("duplicate module identity accepted")}
 invalidIcon:=full();invalidIcon.Modules[0].Icon="<svg>"
 if _,ok:=normalizeFamilyAppearance(invalidIcon);ok{t.Fatal("arbitrary HTML icon accepted")}
 invalidLabel:=full();invalidLabel.Modules[0].Label="\n"
 if _,ok:=normalizeFamilyAppearance(invalidLabel);ok{t.Fatal("empty or control label accepted")}
 long:=full();long.Modules[0].Label="123456789012345678901234567890123"
 if _,ok:=normalizeFamilyAppearance(long);ok{t.Fatal("excessive label accepted")}
}

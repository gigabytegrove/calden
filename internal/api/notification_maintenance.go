package api

import (
 "context"
 "log"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Background review generation is server-initiated. Devices need not open the
// app to cause due/outstanding bill notices to enter the durable push queue.
func StartNotificationMaintenance(ctx context.Context, db *pgxpool.Pool) {
 go func(){
   tick:=time.NewTicker(2*time.Minute)
   defer tick.Stop()
   runNotificationMaintenance(ctx,db)
   for {
     select {
     case <-ctx.Done():return
     case <-tick.C:runNotificationMaintenance(ctx,db)
     }
   }
 }()
}
func runNotificationMaintenance(ctx context.Context,db *pgxpool.Pool){
 scoped,cancel:=context.WithTimeout(ctx,50*time.Second)
 defer cancel()
 rows,err:=db.Query(scoped,`SELECT id,role FROM users WHERE active=true ORDER BY id LIMIT 1000`)
 if err!=nil{log.Printf("Calden notification maintenance: load users failed: %v",err);return}
 type recipient struct{id uuid.UUID;role string}
 people:=[]recipient{}
 for rows.Next(){var person recipient
   if rows.Scan(&person.id,&person.role)==nil{people=append(people,person)}}
 err=rows.Err();rows.Close()
 if err!=nil{log.Printf("Calden notification maintenance: scan failed: %v",err);return}
 engine:=&server{db:db}
 for _,person:=range people {
   if scoped.Err()!=nil{return}
   engine.ensureBillReviewNotifications(scoped,person.id,person.role)
 }
}

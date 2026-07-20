package main

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/lib/pq"
)

func main() {
    dsn := "postgres://scoutmark_ba:feta86v189t3N47@127.0.0.1:15432/scoutmark_ba?sslmode=disable"
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    rows, err := db.Query(`SELECT u.id, u.username, u.display_name, COALESCE(sc.name,'-') AS subcamp, u.is_admin, u.password_change_required
FROM users u
LEFT JOIN subcamps sc ON sc.id = u.subcamp_id
ORDER BY u.created_at ASC, u.username ASC`)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()

    for rows.Next() {
        var id, username, displayName, subcamp string
        var isAdmin, pwdChangeRequired bool
        if err := rows.Scan(&id, &username, &displayName, &subcamp, &isAdmin, &pwdChangeRequired); err != nil {
            log.Fatal(err)
        }
        fmt.Printf("%s\t%s\t%s\t%s\t%t\t%t\n", id, username, displayName, subcamp, isAdmin, pwdChangeRequired)
    }
    if err := rows.Err(); err != nil {
        log.Fatal(err)
    }
}

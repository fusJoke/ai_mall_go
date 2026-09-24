// Command seed 一次性插入一条管理员账号，用于本地 / CI 冒烟测试登录链路。
//
// 用法：go run ./cmd/seed
//
// 不是生产脚本，只是开发辅助。
package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
)

func main() {
	if err := config.Init("."); err != nil {
		log.Fatalf("init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("init database: %v", err)
	}
	defer database.Reset()

	db := database.Get()

	const (
		username = "root"
		password = "Passw0rd!"
		nickname = "超级管理员"
	)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}

	// 用 Username 反查，已有就更新密码，没有就插。
	var existing model.Admin
	err = db.Where("username = ?", username).First(&existing).Error
	now := time.Now()
	switch {
	case err == nil:
		existing.Password = string(hash)
		existing.Status = 1
		existing.LoginFailure = 0
		existing.UpdatedAt = now
		if err := db.Save(&existing).Error; err != nil {
			log.Fatalf("update: %v", err)
		}
		fmt.Printf("updated admin id=%d username=%s\n", existing.ID, existing.Username)
	default:
		adm := model.Admin{
			Username: username,
			Password: string(hash),
			Nickname: nickname,
			Status:   1,
		}
		adm.CreatedAt = now
		adm.UpdatedAt = now
		if err := db.Create(&adm).Error; err != nil {
			log.Fatalf("create: %v", err)
		}
		fmt.Printf("created admin id=%d username=%s\n", adm.ID, adm.Username)
	}
}

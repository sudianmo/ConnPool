package myfunc

import (
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-redis/redis/v8"

	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateStudent(c *gin.Context) {
	var student Student

	ctx := c.Request.Context()

	if err := c.ShouldBindJSON(&student); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "输入格式有误"})
		return
	}

	// 使用连接池获取连接
	db, err := ConnPool.GetConnection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database connection"})
		return
	}
	defer ConnPool.returnConnection(db)

	if err := db.Create(&student).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	datamessage := fmt.Sprintf("%s %d", student.Name, student.Id)

	key := fmt.Sprintf("student:%s", student.Name)
	data, err := json.Marshal(student)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "编码失败"})
		return
	}
	setResult := rdb.Set(ctx, key, string(data), 0)
	if setResult.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "新增redis缓存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"数据新增成功": datamessage})

	if err := UpdateAllStudentsCache(c); err == nil {
		fmt.Println("学生列表缓存已经更新")
	}

}

func GetStudentByName(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	cacheKey := fmt.Sprintf("student:%s", name) // 修复缓存键
	cacheResult, err := rdb.Get(ctx, cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			fmt.Println("没有找到这个键，从数据库查询")
		} else {
			fmt.Println("该条学生缓存获取失败")
		}

		// 从数据库查询
		db, err := ConnPool.GetConnection()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database connection"})
			return
		}
		defer ConnPool.returnConnection(db)

		var student Student
		result := db.Where("name = ?", name).First(&student)
		if result.Error != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "学生不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": student, "source": "database"})

		// 更新缓存
		data, _ := json.Marshal(student)
		rdb.Set(ctx, cacheKey, string(data), 0)
		return
	}

	var student Student
	if err := json.Unmarshal([]byte(cacheResult), &student); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": student, "source": "cache"})
}

func GetStudents(c *gin.Context) {
	var students []Student
	ctx := c.Request.Context()

	cacheKey := "students:all"
	cacheResult, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		if err := json.Unmarshal([]byte(cacheResult), &students); err == nil {
			c.JSON(http.StatusOK, gin.H{"data": students, "source": "cache"})
			return
		}
	}

	// 从数据库查询
	db, err := ConnPool.GetConnection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database connection"})
		return
	}
	defer ConnPool.returnConnection(db)

	result := db.Find(&students)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据获取失败"})
		return
	}

	// 更新缓存
	UpdateAllStudentsCache(c)
	c.JSON(http.StatusOK, gin.H{"data": students, "source": "database"})

	// 显示连接池状态
	active, idle := ConnPool.GetStatus()
	fmt.Printf("连接池状态 - 借出连接数: %d, 空闲连接数: %d\n", active, idle)
}

func UpdateStudent(c *gin.Context) {
	name := c.Param("name")
	var student Student

	if err := c.ShouldBindJSON(&student); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "数据格式有误"})
		return
	}

	// 使用连接池获取连接
	db, err := ConnPool.GetConnection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database connection"})
		return
	}
	defer ConnPool.returnConnection(db)

	result := db.Where("name = ?", name).Updates(&student)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取行数失败"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "数据操作失败，请检查姓名是否输入错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})

	// 更新缓存
	err = UpdateAllStudentsCache(c)
	if err != nil {
		fmt.Println("更新缓存失败")
	}
	deleteStudentCache(c, name)

	key := fmt.Sprintf("student:%s", student.Name)
	data, err := json.Marshal(student)
	if err != nil {
		fmt.Println("更新单个学生缓存失败")
	} else {
		rdb.Set(c, key, string(data), 0)
	}

	// 显示连接池状态
	active, idle := ConnPool.GetStatus()
	fmt.Printf("连接池状态 - 借出连接数: %d, 空闲连接数: %d\n", active, idle)
}

func DeleteStudent(c *gin.Context) {
	name := c.Param("name")

	// 使用连接池获取连接
	db, err := ConnPool.GetConnection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database connection"})
		return
	}
	defer ConnPool.returnConnection(db)

	result := db.Where("name = ?", name).Delete(&Student{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器操作失败"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "数据操作失败，请检查姓名是否输入错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"操作成功": "删除学生"})
	deleteStudentCache(c, name)

	err = UpdateAllStudentsCache(c)
	if err != nil {
		fmt.Println("更新缓存失败")
	}

	// 显示连接池状态
	active, idle := ConnPool.GetStatus()
	fmt.Printf("连接池状态 - 借出连接数: %d, 空闲连接数: %d\n", active, idle)
}

func UpdateAllStudentsCache(c *gin.Context) error {
	// 使用连接池获取连接
	db, err := ConnPool.GetConnection()
	if err != nil {
		return err
	}
	defer ConnPool.returnConnection(db)

	var students []Student
	result := db.Find(&students)
	if result.Error != nil {
		return result.Error
	}

	studentsCache, err := json.Marshal(students)
	if err != nil {
		return err
	}

	key := "students:all"
	return rdb.Set(c, key, studentsCache, 0).Err()
}

func deleteStudentCache(ctx *gin.Context, name string) {
	key := fmt.Sprintf("student:%s", name)
	err := rdb.Del(ctx, key).Err()
	if err != nil {
		fmt.Println("删除缓存失败：", err)
		return
	}
	fmt.Println("成功删除键：", name)
}

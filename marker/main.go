package main

import (
	"os"
    "io/ioutil"
	"errors"
	"net/http"
	"strings"
	"time"
    "fmt"
    "encoding/json"
    "strconv"

	"github.com/joho/godotenv"
	"github.com/google/uuid"
)

type AccessToken struct {
    Value string `json:"access_token"`
    ExpiresAt   int64  `json:"expires_at"`
}

func (a *AccessToken) ok() bool {
	return time.Now().Unix() <= a.ExpiresAt
}

func checkEnv() bool {
    envExpiresAt := os.Getenv("EXPIREDAT")
    envValue := os.Getenv("VALUE")

    if envExpiresAt == "" || envValue == "" {
        fmt.Errorf("EXPIREDAT or VALUE is empty")
        return false
    }

    return true
}

func getAccess(key string) (*AccessToken, error) {
    /*
    scenario

    надо сначала проверить, может у меня есть access в переменной окружения
    если есть, то валидный ли он?
    если нет, то создать и записать в переменную окружения
    */
    if checkEnv() {
        expiresAt, err := strconv.ParseInt(os.Getenv("EXPIREDAT"), 10, 64)
        if err != nil {
            fmt.Printf("Ошибка парсинга EXPIREDAT: %v\n", err)
            expiresAt = 0
        }

        token := &AccessToken{
            Value: os.Getenv("VALUE"),
            ExpiresAt: expiresAt,
        }

        if token.ok() {
            return token, nil
        }
    }

    uuid4, err := uuid.NewRandom()
    if err != nil {
        return nil, err
    }

    payload := strings.NewReader("scope=GIGACHAT_API_PERS")
    url := os.Getenv("URL")
    method := "POST"

    client := &http.Client{Timeout: 10 * time.Second}

    req, err := http.NewRequest(method, url, payload)
    if err != nil {
        return nil, err
    }

    req.Header.Add("RqUID", uuid4.String())
    req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", "Basic " + os.Getenv("AUTH_KEY"))

    // fmt.Println("======== REQUEST ========")
    // fmt.Println("URL:", url)
    // fmt.Println("Headers:")
    // for name, values := range req.Header {
    //     fmt.Printf("  %s: %s\n", name, strings.Join(values, ", "))
    // }

    // bodyCopy, _ := ioutil.ReadAll(req.Body)
    // fmt.Println("Body:", string(bodyCopy))
    // // восстановим body (ReadAll его "съел")
    // req.Body = ioutil.NopCloser(strings.NewReader("scope=GIGACHAT_API_PERS"))
    // fmt.Println("=========================")

    res, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()

    bodyBytes, err := ioutil.ReadAll(res.Body)
    if err != nil {
        return nil, errors.New("ошибка при чтении тела ответа")
    }

    var response AccessToken
    err = json.Unmarshal(bodyBytes, &response)

    // fmt.Println("==== RAW RESPONSE ====")
    // fmt.Println(string(bodyBytes))
    // fmt.Println("======================")

    if err != nil {
        return nil, errors.New("ошибка при парсинге JSON")
    }

    envMap, _ := godotenv.Read(".env")
    envMap["EXPIREDAT"] = strconv.FormatInt(response.ExpiresAt, 10)
    envMap["VALUE"] = response.Value
    err = godotenv.Write(envMap, ".env")
    if err != nil {
        return nil, err
    }

    return &response, nil
}

// func sendPic(filename string) (*http.Response, error) {
// 	f, err := os.Open(filename)
// 	if err != nil {
// 		// fmt.Errorf("Ошибка: %v", err)
// 		return nil, err
// 	}
//     defer f.Close()
//     //TODO
// }

func main() {
	err := godotenv.Load(".env")
    if err != nil {
        fmt.Printf("godotenv.Load() err: %v\n", err)
        return
    }

    k := os.Getenv("AUTH_KEY")
    if k == "" {
        fmt.Printf("key is empty")
        return
    }

    t, err := getAccess(k)
    if err != nil {
        fmt.Printf("getAccess error: %v\n", err)
        return
    }

    fmt.Printf("Access token: %s\nExpires at: %d\n", t.Value, t.ExpiresAt)
}
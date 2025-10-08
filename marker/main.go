package main

import (
    "os"
    "io"
    "errors"
    "net/http"
    "strings"
    "time"
    "fmt"
    "encoding/json"
    "strconv"
    "bytes"
    "mime"
    "mime/multipart"
    "net/textproto"
    "path/filepath"

    "github.com/joho/godotenv"
    "github.com/google/uuid"
)


const maxSizeByte = 15 * 1024 * 1024 // 15MB

type MarkerResponse struct {
    Gender string `json:"gender"`
    Category string `json:"category"`
    Style string `json:"style"`
    Colour string `json:"colour"`
    Season string `json:"season"`
    Material string `json:"material"`
}
 
type Img struct {
    Bytes int64 `json:"bytes"`
    CreatedAt int64 `json:"created_at"`
    Name string `json:"filename"`
    ID uuid.UUID `json:"id"`
    Type string `json:"object"`
    Purpose string `json:"purpose"`
    AccessPolicy string `json:"access_policy"`
}

type AccessToken struct {
    Value string `json:"access_token"`
    ExpiresAt   int64  `json:"expires_at"`
}

func (a *AccessToken) ok() bool {
	return time.Now().Unix() <= a.ExpiresAt
}

func checkEnv() bool {
    envExpiresAt := os.Getenv("ACCESSEXPIREDAT")
    envValue := os.Getenv("ACCESSVALUE")

    if envExpiresAt == "" || envValue == "" {
        fmt.Println("ACCESSEXPIREDAT or ACCESSVALUE is empty")
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
        expiresAt, err := strconv.ParseInt(os.Getenv("ACCESSEXPIREDAT"), 10, 64)
        if err != nil {
            fmt.Printf("Ошибка парсинга ACCESSEXPIREDAT: %v\n", err)
            expiresAt = 0
        }

        token := &AccessToken{
            Value: os.Getenv("ACCESSVALUE"),
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
    url := os.Getenv("ACCESSURL")

    client := &http.Client{Timeout: 10 * time.Second}

    req, err := http.NewRequest("POST", url, payload)
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

    // bodyCopy, _ := io.ioutil.ReadAll(req.Body)
    // fmt.Println("Body:", string(bodyCopy))
    // // восстановим body (ReadAll его "съел")
    // req.Body = io.ioutil.NopCloser(strings.NewReader("scope=GIGACHAT_API_PERS"))
    // fmt.Println("=========================")

    res, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()

    bodyBytes, err := io.ReadAll(res.Body)
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
    envMap["ACCESSEXPIREDAT"] = strconv.FormatInt(response.ExpiresAt, 10)
    envMap["ACCESSVALUE"] = response.Value
    err = godotenv.Write(envMap, ".env")
    if err != nil {
        return nil, err
    }

    return &response, nil
}

func loadPic(filename string) (*Img, error) {
    filePath := "pics/"+filename

    fileInfo, err := os.Stat(filePath)
    if err != nil {
        return nil, err
    }

    if fileInfo.Size() > maxSizeByte {
        return nil, fmt.Errorf("превышен максимальный размер картинки")
    }

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
    defer f.Close()

    ext := strings.ToLower(filepath.Ext(filename))
    mimeType := mime.TypeByExtension(ext)
    if mimeType == "" {
        return nil, fmt.Errorf("неподдерживаемый формат файла: %s", ext)
    }

    var b bytes.Buffer
    writer := multipart.NewWriter(&b)

    partHeader := textproto.MIMEHeader{}
    partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", fileInfo.Name()))
    partHeader.Set("Content-Type", mimeType)

    part, err := writer.CreatePart(partHeader)
    if err != nil {
        return nil, err
    }

    if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}

    if err = writer.WriteField("purpose", "general"); err != nil {
		return nil, err
	}
	writer.Close()

    client := &http.Client{Timeout: 30 * time.Second}
    url := os.Getenv("LOADURL")
    
    req, err := http.NewRequest("POST", url, &b)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Authorization", "Bearer " + os.Getenv("ACCESSVALUE"))

    res, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()

    bodyBytes, err := io.ReadAll(res.Body)
    if err != nil {
        return nil, errors.New("ошибка при чтении тела ответа")
    }

    // fmt.Println("==== RAW RESPONSE ====")
    // fmt.Println(string(bodyBytes))
    // fmt.Println("======================")

    var response Img
    if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("ошибка при парсинге JSON: %v", err)
	}

    return &response, nil
}

func sendPrompt(fileID string) (*MarkerResponse, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	url := os.Getenv("PROMPTURL")

	systemPrompt := `You are a professional fashion expert. You have a clothing description.
        Create the most detailed JSON possible with the following keys:
        'gender' — gender (e.g., male, female, unisex)
        'category' — clothing category (e.g., t-shirt, pants, jacket)
        'style' — style (e.g., casual, street, sport, classic)
        'color' — color (main and additional, if any)
        'season' — season (e.g., summer, autumn, winter, spring)
        'material' — material (e.g., cotton, polyester, wool). 
        Return only valid JSON without explanations.`

	requestBody := map[string]interface{}{
		"model": "GigaChat",
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":        "user",
				"content":     "Please analyze this image and return JSON description.",
				"attachments": []string{fileID},
			},
		},
		"temperature":       0.3,
		"top_p":             0.9,
		"stream":            false,
		"update_interval":   0,
		"repetition_penalty": 1.0,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации тела запроса: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("ACCESSVALUE"))

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка при выполнении запроса: %v", err)
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела ответа: %v", err)
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("ошибка API: %s", string(respBody))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON-ответа: %v", err)
	}

	messages, ok := raw["choices"].([]interface{})
	if !ok || len(messages) == 0 {
		return nil, fmt.Errorf("в ответе нет поля choices")
	}

	message := messages[0].(map[string]interface{})
	content := message["message"].(map[string]interface{})["content"].(string)

	var parsed MarkerResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON контента: %v\n%s", err, content)
	}

	return &parsed, nil
}


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

    // img, err := loadPic("blouse1.jpeg")
    // if err != nil {
    //     fmt.Printf("loadPic error: %v\n", err)
    //     return
    // }

    // result, err := sendPrompt(img.ID.String())
    // if err != nil {
    //     fmt.Printf("sendPrompt error: %v\n", err)
    //     return
    // }

    // fmt.Printf("Результат анализа: %+v\n", result)
}
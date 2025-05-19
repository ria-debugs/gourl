package main

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"
    "os"

    "github.com/skip2/go-qrcode"
)

type URLInfo struct {
    OriginalURL string
    Expiry      time.Time
}

var (
    store   = make(map[string]URLInfo)
    storeMu sync.RWMutex

    templates = template.Must(template.ParseFiles("templates/index.html"))
)

type PageData struct {
    Error         string
    ShortURL      string
	QRCodeBase64  template.URL
    Expiry        string
    DefaultExpiry string
    OriginalURL   string
    CustomCode    string
}

func main() {
    http.HandleFunc("/", homeHandler)
    http.HandleFunc("/shorten", shortenHandler)
    http.HandleFunc("/r/", redirectHandler)

    fmt.Println("Server started at http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(w, r)
        return
    }

    oneMonthLater := time.Now().AddDate(0, 1, 0)
    defaultExpiry := oneMonthLater.Format("2006-01-02T15:04")

    data := PageData{
        DefaultExpiry: defaultExpiry,
    }
    err := templates.ExecuteTemplate(w, "index.html", data)
    if err != nil {
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
}

func shortenHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    originalURL := strings.TrimSpace(r.FormValue("url"))
    customCode := strings.TrimSpace(r.FormValue("customcode"))
    expiryStr := r.FormValue("expiry")

    data := PageData{
        OriginalURL: originalURL,
        CustomCode:  customCode,
        Expiry:      expiryStr,
    }

    if originalURL == "" {
        data.Error = "Please enter a URL."
        renderTemplate(w, data)
        return
    }
    if !isValidURL(originalURL) {
        data.Error = "Invalid URL format."
        renderTemplate(w, data)
        return
    }

    if containsProfanity(originalURL) || containsProfanity(customCode) {
        data.Error = "Inappropriate language detected in URL or custom code."
        renderTemplate(w, data)
        return
    }

    var expiry time.Time
    if expiryStr == "" {
        expiry = time.Now().AddDate(0, 1, 0)
    } else {
        t, err := time.Parse("2006-01-02T15:04", expiryStr)
        if err != nil {
            data.Error = "Invalid expiry date format."
            renderTemplate(w, data)
            return
        }
        expiry = t
    }
    if expiry.Before(time.Now()) {
        data.Error = "Expiry date must be in the future."
        renderTemplate(w, data)
        return
    }

    shortCode := customCode
    if shortCode == "" {
        var err error
        shortCode, err = generateShortCode(6)
        if err != nil {
            data.Error = "Error generating short code."
            renderTemplate(w, data)
            return
        }
    } else {
        storeMu.RLock()
        _, exists := store[shortCode]
        storeMu.RUnlock()
        if exists {
            data.Error = "Custom short code already taken."
            renderTemplate(w, data)
            return
        }
    }

    storeMu.Lock()
    store[shortCode] = URLInfo{
        OriginalURL: originalURL,
        Expiry:      expiry,
    }
    storeMu.Unlock()

    shortURL := fmt.Sprintf("%s/r/%s", getBaseURL(r), shortCode)
    data.ShortURL = shortURL

    png, err := qrcode.Encode(shortURL, qrcode.Medium, 256)
    if err == nil {
		data.QRCodeBase64 = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))

    }

    renderTemplate(w, data)
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
    code := strings.TrimPrefix(r.URL.Path, "/r/")
    storeMu.RLock()
    urlInfo, exists := store[code]
    storeMu.RUnlock()

    if !exists {
        http.NotFound(w, r)
        return
    }

    if time.Now().After(urlInfo.Expiry) {
        http.Error(w, "This link has expired.", http.StatusGone)
        return
    }

    http.Redirect(w, r, urlInfo.OriginalURL, http.StatusFound)
}

func renderTemplate(w http.ResponseWriter, data PageData) {
    err := templates.ExecuteTemplate(w, "index.html", data)
    if err != nil {
        http.Error(w, "Internal server error", http.StatusInternalServerError)
    }
}

func isValidURL(str string) bool {
    u, err := url.ParseRequestURI(str)
    return err == nil && u.Scheme != "" && u.Host != ""
}

func generateShortCode(n int) (string, error) {
    b := make([]byte, n)
    _, err := rand.Read(b)
    if err != nil {
        return "", err
    }
    s := base64.URLEncoding.EncodeToString(b)
    return s[:n], nil
}

func containsProfanity(text string) bool {
    profanities := []string{"badword1", "badword2", "badword3"}
    textLower := strings.ToLower(text)
    for _, p := range profanities {
        if strings.Contains(textLower, p) {
            return true
        }
    }
    return false
}

func getBaseURL(r *http.Request) string {
   

    baseURL := os.Getenv("BASE_URL")
    if baseURL == "" {
        // local
        baseURL = "http://localhost:8080"
    }
    return baseURL
   // scheme := "http"
   // if r.TLS != nil {
   //     scheme = "https"
   // }
   // return scheme + "://" + r.Host
   //return "https://gourl.onrender.com"
}

module openrisk/backend

go 1.25.4

require (
	github.com/gin-contrib/cors v1.7.2
	github.com/gin-contrib/ratelimit v0.0.0-20240329003816-5f02409f42e0
	github.com/gin-gonic/gin v1.10.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/rs/zerolog v1.33.0
	gorm.io/driver/postgres v1.5.9
	gorm.io/gorm v1.25.12
	github.com/jung-kurt/gofpdf v1.0.3-0.20190309125859-24315acbbda5  // PDF exports
	github.com/signintech/gopdf v0.28.0  // Alternative PDF if needed
	golang.org/x/time v0.7.1  // Rate limiting
)

require (
	github.com/bytedance/sonic v1.11.3 // indirect
	github.com/bytedance/sonic/loader v0.1.1 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.20.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606112141-5926e8d38f4a // indirect
	github.com/jackc/pgx/v5 v5.6.0 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.25.0 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
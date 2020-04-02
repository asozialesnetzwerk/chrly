package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/thedevsaddam/govalidator"

	"github.com/elyby/chrly/api/mojang"
	"github.com/elyby/chrly/model"
)

//noinspection GoSnakeCaseUsage
const UUID_ANY = "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

var regexUuidAny = regexp.MustCompile(UUID_ANY)

func init() {
	govalidator.AddCustomRule("skinUploadingNotAvailable", func(field string, rule string, message string, value interface{}) error {
		if message == "" {
			message = "Skin uploading is temporary unavailable"
		}

		return errors.New(message)
	})

	// Add ability to validate any possible uuid form
	govalidator.AddCustomRule("uuid_any", func(field string, rule string, message string, value interface{}) error {
		str := value.(string)
		if !regexUuidAny.MatchString(str) {
			if message == "" {
				message = fmt.Sprintf("The %s field must contain valid UUID", field)
			}

			return errors.New(message)
		}

		return nil
	})
}

type SkinsRepository interface {
	FindByUsername(username string) (*model.Skin, error)
	FindByUserId(id int) (*model.Skin, error)
	Save(skin *model.Skin) error
	RemoveByUserId(id int) error
	RemoveByUsername(username string) error
}

type CapesRepository interface {
	FindByUsername(username string) (*model.Cape, error)
}

type SkinNotFoundError struct {
	Who string
}

func (e SkinNotFoundError) Error() string {
	return "skin data not found"
}

type CapeNotFoundError struct {
	Who string
}

func (e CapeNotFoundError) Error() string {
	return "cape file not found"
}

type MojangTexturesProvider interface {
	GetForUsername(username string) (*mojang.SignedTexturesResponse, error)
}

type Skinsystem struct {
	Emitter
	TexturesExtraParamName  string
	TexturesExtraParamValue string
	SkinsRepo               SkinsRepository
	CapesRepo               CapesRepository
	MojangTexturesProvider  MojangTexturesProvider
	Authenticator           Authenticator
}

func (ctx *Skinsystem) CreateHandler() *mux.Router {
	requestEventsMiddleware := CreateRequestEventsMiddleware(ctx.Emitter, "skinsystem")

	router := mux.NewRouter().StrictSlash(true)
	router.Use(requestEventsMiddleware)

	router.HandleFunc("/skins/{username}", ctx.Skin).Methods(http.MethodGet)
	router.HandleFunc("/cloaks/{username}", ctx.Cape).Methods(http.MethodGet).Name("cloaks")
	router.HandleFunc("/textures/{username}", ctx.Textures).Methods(http.MethodGet)
	router.HandleFunc("/textures/signed/{username}", ctx.SignedTextures).Methods(http.MethodGet)
	// Legacy
	router.HandleFunc("/skins", ctx.SkinGET).Methods(http.MethodGet)
	router.HandleFunc("/cloaks", ctx.CapeGET).Methods(http.MethodGet)
	// API
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Use(CreateAuthenticationMiddleware(ctx.Authenticator))
	apiRouter.HandleFunc("/skins", ctx.PostSkin).Methods(http.MethodPost)
	apiRouter.HandleFunc("/skins/id:{id:[0-9]+}", ctx.DeleteSkinByUserId).Methods(http.MethodDelete)
	apiRouter.HandleFunc("/skins/{username}", ctx.DeleteSkinByUsername).Methods(http.MethodDelete)
	// 404
	// NotFoundHandler doesn't call for registered middlewares, so we must wrap it manually.
	// See https://github.com/gorilla/mux/issues/416#issuecomment-600079279
	router.NotFoundHandler = requestEventsMiddleware(http.HandlerFunc(NotFound))

	return router
}

func (ctx *Skinsystem) Skin(response http.ResponseWriter, request *http.Request) {
	username := parseUsername(mux.Vars(request)["username"])
	rec, err := ctx.SkinsRepo.FindByUsername(username)
	if err == nil && rec.SkinId != 0 {
		http.Redirect(response, request, rec.Url, 301)
		return
	}

	mojangTextures, err := ctx.MojangTexturesProvider.GetForUsername(username)
	if err != nil || mojangTextures == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	texturesProp := mojangTextures.DecodeTextures()
	skin := texturesProp.Textures.Skin
	if skin == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	http.Redirect(response, request, skin.Url, 301)
}

func (ctx *Skinsystem) SkinGET(response http.ResponseWriter, request *http.Request) {
	username := request.URL.Query().Get("name")
	if username == "" {
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	mux.Vars(request)["username"] = username
	mux.Vars(request)["converted"] = "1"

	ctx.Skin(response, request)
}

func (ctx *Skinsystem) Cape(response http.ResponseWriter, request *http.Request) {
	username := parseUsername(mux.Vars(request)["username"])
	rec, err := ctx.CapesRepo.FindByUsername(username)
	if err == nil {
		request.Header.Set("Content-Type", "image/png")
		_, _ = io.Copy(response, rec.File)
		return
	}

	mojangTextures, err := ctx.MojangTexturesProvider.GetForUsername(username)
	if err != nil || mojangTextures == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	texturesProp := mojangTextures.DecodeTextures()
	cape := texturesProp.Textures.Cape
	if cape == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	http.Redirect(response, request, cape.Url, 301)
}

func (ctx *Skinsystem) CapeGET(response http.ResponseWriter, request *http.Request) {
	username := request.URL.Query().Get("name")
	if username == "" {
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	mux.Vars(request)["username"] = username
	mux.Vars(request)["converted"] = "1"

	ctx.Cape(response, request)
}

func (ctx *Skinsystem) Textures(response http.ResponseWriter, request *http.Request) {
	username := parseUsername(mux.Vars(request)["username"])

	var textures *mojang.TexturesResponse
	skin, skinErr := ctx.SkinsRepo.FindByUsername(username)
	_, capeErr := ctx.CapesRepo.FindByUsername(username)
	if (skinErr == nil && skin.SkinId != 0) || capeErr == nil {
		textures = &mojang.TexturesResponse{}

		if skinErr == nil && skin.SkinId != 0 {
			skinTextures := &mojang.SkinTexturesResponse{
				Url: skin.Url,
			}

			if skin.IsSlim {
				skinTextures.Metadata = &mojang.SkinTexturesMetadata{
					Model: "slim",
				}
			}

			textures.Skin = skinTextures
		}

		if capeErr == nil {
			textures.Cape = &mojang.CapeTexturesResponse{
				Url: request.URL.Scheme + "://" + request.Host + "/cloaks/" + username,
			}
		}
	} else {
		mojangTextures, err := ctx.MojangTexturesProvider.GetForUsername(username)
		if err != nil || mojangTextures == nil {
			response.WriteHeader(http.StatusNoContent)
			return
		}

		texturesProp := mojangTextures.DecodeTextures()
		if texturesProp == nil {
			ctx.Emit("skinsystem:error", errors.New("unable to find textures property"))
			apiServerError(response)
			return
		}

		textures = texturesProp.Textures
	}

	responseData, _ := json.Marshal(textures)
	response.Header().Set("Content-Type", "application/json")
	_, _ = response.Write(responseData)
}

func (ctx *Skinsystem) SignedTextures(response http.ResponseWriter, request *http.Request) {
	username := parseUsername(mux.Vars(request)["username"])

	var responseData *mojang.SignedTexturesResponse

	rec, err := ctx.SkinsRepo.FindByUsername(username)
	if err == nil && rec.SkinId != 0 && rec.MojangTextures != "" {
		responseData = &mojang.SignedTexturesResponse{
			Id:   strings.Replace(rec.Uuid, "-", "", -1),
			Name: rec.Username,
			Props: []*mojang.Property{
				{
					Name:      "textures",
					Signature: rec.MojangSignature,
					Value:     rec.MojangTextures,
				},
			},
		}
	} else if request.URL.Query().Get("proxy") != "" {
		mojangTextures, err := ctx.MojangTexturesProvider.GetForUsername(username)
		if err == nil && mojangTextures != nil {
			responseData = mojangTextures
		}
	}

	if responseData == nil {
		response.WriteHeader(http.StatusNoContent)
		return
	}

	responseData.Props = append(responseData.Props, &mojang.Property{
		Name:  getStringOrDefault(ctx.TexturesExtraParamName, "chrly"),
		Value: getStringOrDefault(ctx.TexturesExtraParamValue, "how do you tame a horse in Minecraft?"),
	})

	responseJson, _ := json.Marshal(responseData)
	response.Header().Set("Content-Type", "application/json")
	_, _ = response.Write(responseJson)
}

func (ctx *Skinsystem) PostSkin(resp http.ResponseWriter, req *http.Request) {
	validationErrors := validatePostSkinRequest(req)
	if validationErrors != nil {
		apiBadRequest(resp, validationErrors)
		return
	}

	identityId, _ := strconv.Atoi(req.Form.Get("identityId"))
	username := req.Form.Get("username")

	record, err := findIdentity(ctx.SkinsRepo, identityId, username)
	if err != nil {
		ctx.Emit("skinsystem:error", fmt.Errorf("error on requesting a skin from the repository: %w", err))
		apiServerError(resp)
		return
	}

	skinId, _ := strconv.Atoi(req.Form.Get("skinId"))
	is18, _ := strconv.ParseBool(req.Form.Get("is1_8"))
	isSlim, _ := strconv.ParseBool(req.Form.Get("isSlim"))

	record.Uuid = req.Form.Get("uuid")
	record.SkinId = skinId
	record.Is1_8 = is18
	record.IsSlim = isSlim
	record.Url = req.Form.Get("url")
	record.MojangTextures = req.Form.Get("mojangTextures")
	record.MojangSignature = req.Form.Get("mojangSignature")

	err = ctx.SkinsRepo.Save(record)
	if err != nil {
		ctx.Emit("skinsystem:error", fmt.Errorf("unable to save record to the repository: %w", err))
		apiServerError(resp)
		return
	}

	resp.WriteHeader(http.StatusCreated)
}

func (ctx *Skinsystem) DeleteSkinByUserId(resp http.ResponseWriter, req *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(req)["id"])
	skin, err := ctx.SkinsRepo.FindByUserId(id)
	ctx.deleteSkin(skin, err, resp)
}

func (ctx *Skinsystem) DeleteSkinByUsername(resp http.ResponseWriter, req *http.Request) {
	username := mux.Vars(req)["username"]
	skin, err := ctx.SkinsRepo.FindByUsername(username)
	ctx.deleteSkin(skin, err, resp)
}

func (ctx *Skinsystem) deleteSkin(skin *model.Skin, err error, resp http.ResponseWriter) {
	if err != nil {
		if _, ok := err.(*SkinNotFoundError); ok {
			apiNotFound(resp, "Cannot find record for the requested identifier")
		} else {
			ctx.Emit("skinsystem:error", fmt.Errorf("unable to find skin info from the repository: %w", err))
			apiServerError(resp)
		}

		return
	}

	err = ctx.SkinsRepo.RemoveByUserId(skin.UserId)
	if err != nil {
		ctx.Emit("skinsystem:error", fmt.Errorf("cannot delete skin by error: %w", err))
		apiServerError(resp)
		return
	}

	resp.WriteHeader(http.StatusNoContent)
}

func validatePostSkinRequest(request *http.Request) map[string][]string {
	const maxMultipartMemory int64 = 32 << 20
	const oneOfSkinOrUrlMessage = "One of url or skin should be provided, but not both"

	_ = request.ParseMultipartForm(maxMultipartMemory)

	validationRules := govalidator.MapData{
		"identityId": {"required", "numeric", "min:1"},
		"username":   {"required"},
		"uuid":       {"required", "uuid_any"},
		"skinId":     {"required", "numeric", "min:1"},
		"url":        {"url"},
		"file:skin":  {"ext:png", "size:24576", "mime:image/png"},
		"is1_8":      {"bool"},
		"isSlim":     {"bool"},
	}

	shouldAppendSkinRequiredError := false
	url := request.Form.Get("url")
	_, _, skinErr := request.FormFile("skin")
	if (url != "" && skinErr == nil) || (url == "" && skinErr != nil) {
		shouldAppendSkinRequiredError = true
	} else if skinErr == nil {
		validationRules["file:skin"] = append(validationRules["file:skin"], "skinUploadingNotAvailable")
	} else if url != "" {
		validationRules["is1_8"] = append(validationRules["is1_8"], "required")
		validationRules["isSlim"] = append(validationRules["isSlim"], "required")
	}

	mojangTextures := request.Form.Get("mojangTextures")
	if mojangTextures != "" {
		validationRules["mojangSignature"] = []string{"required"}
	}

	validator := govalidator.New(govalidator.Options{
		Request:         request,
		Rules:           validationRules,
		RequiredDefault: false,
		FormSize:        maxMultipartMemory,
	})
	validationResults := validator.Validate()
	if shouldAppendSkinRequiredError {
		validationResults["url"] = append(validationResults["url"], oneOfSkinOrUrlMessage)
		validationResults["skin"] = append(validationResults["skin"], oneOfSkinOrUrlMessage)
	}

	if len(validationResults) != 0 {
		return validationResults
	}

	return nil
}

func findIdentity(repo SkinsRepository, identityId int, username string) (*model.Skin, error) {
	var record *model.Skin
	record, err := repo.FindByUserId(identityId)
	if err != nil {
		if _, isSkinNotFound := err.(*SkinNotFoundError); !isSkinNotFound {
			return nil, err
		}

		record, err = repo.FindByUsername(username)
		if err == nil {
			_ = repo.RemoveByUsername(username)
			record.UserId = identityId
		} else {
			record = &model.Skin{
				UserId:   identityId,
				Username: username,
			}
		}
	} else if record.Username != username {
		_ = repo.RemoveByUserId(identityId)
		record.Username = username
	}

	return record, nil
}

func parseUsername(username string) string {
	return strings.TrimSuffix(username, ".png")
}

func getStringOrDefault(value string, def string) string {
	if value != "" {
		return value
	}

	return def
}

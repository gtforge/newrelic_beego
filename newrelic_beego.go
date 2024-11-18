package newrelic_beego

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/beego/beego"
	"github.com/beego/beego/context"
	newrelicv3 "github.com/newrelic/go-agent/v3/newrelic"
)

var (
	reNumberIDInPath = regexp.MustCompile("[0-9]{2,}")
	reg              = regexp.MustCompile(`[a-zA-Z0-9_]+`)
	NewrelicAgent    *newrelicv3.Application
)

const newRelicSkipPaths = "newrelic_skip_paths"
const newRelicAppName = "appname"
const newRelicLicense = "newrelic_license"

// Paths NewRelic needs to skip from reporting
var skipPaths map[string]bool

func init() {
	appName := os.Getenv("NEW_RELIC_APP_NAME")
	license := os.Getenv("NEW_RELIC_LICENSE_KEY")

	skipPaths = parseSkipPaths(beego.AppConfig.String(newRelicSkipPaths))

	if appName == "" {
		appName = beego.AppConfig.String("newrelic_appname")
	}
	if appName == "" {
		appName = beego.AppConfig.String(newRelicAppName)
	}
	if license == "" {
		license = beego.AppConfig.String(newRelicLicense)
		if license == "" && beego.BConfig.RunMode == "prod" {
			beego.Warn("Please set NewRelic license in config(newrelic_license)")
			return
		}
	}

	app, err := newrelicv3.NewApplication(
		newrelicv3.ConfigAppName(appName),
		newrelicv3.ConfigLicense(license),
		newrelicv3.ConfigInfoLogger(os.Stdout),
		func(c *newrelicv3.Config) {
			c.CrossApplicationTracer.Enabled = false
			c.ErrorCollector.RecordPanics = true

			// Ignore all 4xx errors. By default NR records 4xx errors, see
			// https://github.com/newrelic/go-agent/blob/82c8f8440ca84eb68e08248d877fa1d0b55da333/GUIDE.md?plain=1#L738
			c.ErrorCollector.IgnoreStatusCodes = append(
				c.ErrorCollector.IgnoreStatusCodes,
				http.StatusBadRequest,
				http.StatusUnauthorized,
				http.StatusPaymentRequired,
				http.StatusForbidden,
				// http.StatusNotFound, // already in the list
				http.StatusMethodNotAllowed,
				http.StatusNotAcceptable,
				http.StatusProxyAuthRequired,
				http.StatusRequestTimeout,
				http.StatusConflict,
				http.StatusGone,
				http.StatusLengthRequired,
				http.StatusPreconditionFailed,
				http.StatusRequestEntityTooLarge,
				http.StatusRequestURITooLong,
				http.StatusUnsupportedMediaType,
				http.StatusRequestedRangeNotSatisfiable,
				http.StatusExpectationFailed,
				http.StatusTeapot,
				http.StatusMisdirectedRequest,
				http.StatusUnprocessableEntity,
				http.StatusLocked,
				http.StatusFailedDependency,
				http.StatusTooEarly,
				http.StatusUpgradeRequired,
				http.StatusPreconditionRequired,
				http.StatusTooManyRequests,
				http.StatusRequestHeaderFieldsTooLarge,
				http.StatusUnavailableForLegalReasons,
			)
		},
	)

	if err != nil {
		beego.Warn(err.Error())
		return
	}

	NewrelicAgent = app

	beego.InsertFilter("*", beego.BeforeRouter, StartTransaction, false)
	beego.InsertFilter("*", beego.FinishRouter, EndTransaction, false)
	beego.Info("NewRelic agent started")
}

// parseSkipPaths gets string of comma separated paths
// It returns a set of normalized paths
func parseSkipPaths(pathsConfig string) map[string]bool {
	paths := map[string]bool{}
	splitPaths := strings.Split(pathsConfig, ",")

	for _, path := range splitPaths {
		formatted := strings.TrimSpace(path)
		formatted = strings.ToLower(formatted)
		if formatted != "" {
			paths[formatted] = true
		}
	}

	return paths
}

const newRelicTransaction = "newrelic_transaction"

func StartTransaction(ctx *context.Context) {
	if shouldSkip(skipPaths, ctx.Request.URL.Path) {
		return
	}

	tx := NewrelicAgent.StartTransaction(ctx.Request.URL.Path)

	tx.SetWebRequestHTTP(ctx.Request)

	ctx.ResponseWriter.ResponseWriter = tx.SetWebResponse(ctx.ResponseWriter.ResponseWriter)
	ctx.Input.SetData(newRelicTransaction, tx)
}

// shouldSkip decides if given path matches against declared skip paths
// Support both exact matches and wildcard matches
func shouldSkip(skipPaths map[string]bool, path string) bool {
	_, exactMatch := skipPaths[path]

	if exactMatch {
		return true
	}

	for skipPath := range skipPaths {
		if strings.Contains(path, skipPath) {
			return true
		}
	}

	return false
}

func NameTransaction(ctx *context.Context) {
	var path string
	if ctx.Input.GetData(newRelicTransaction) == nil {
		return
	}
	tx := ctx.Input.GetData(newRelicTransaction).(*newrelicv3.Transaction)
	// in old beego pattern available only in dev mode
	pattern, ok := ctx.Input.GetData("RouterPattern").(string)
	if ok {
		path = generatePath(pattern)
	} else {
		path = reNumberIDInPath.ReplaceAllString(ctx.Request.URL.Path, ":id")
	}

	displayExplicitEnv := beego.AppConfig.DefaultString("newrelic_display_explicit_env", "FALSE")
	if strings.ToUpper(displayExplicitEnv) == "TRUE" {
		env := strings.ToUpper(ctx.Input.Param(":env"))
		path = strings.Replace(path, ":env", env, -1)
	}

	txName := fmt.Sprintf("%s %s", ctx.Request.Method, path)
	tx.SetName(txName)
}

func EndTransaction(ctx *context.Context) {
	NameTransaction(ctx)
	if ctx.Input.GetData(newRelicTransaction) != nil {
		tx := ctx.Input.GetData(newRelicTransaction).(*newrelicv3.Transaction)
		tx.End()
	}
}

func generatePath(pattern string) string {
	segments := splitPath(pattern)
	for i, seg := range segments {
		segments[i] = replaceSegment(seg)
	}
	return strings.Join(segments, "/")
}

func splitPath(key string) []string {
	key = strings.Trim(key, "/ ")
	if key == "" {
		return []string{}
	}
	return strings.Split(key, "/")
}

func replaceSegment(seg string) string {
	colonSlice := []rune{':'}
	if strings.ContainsAny(seg, ":") {
		var newSegment []rune
		var start bool
		var startexp bool
		var param []rune
		var skipnum int
		for i, v := range seg {
			if skipnum > 0 {
				skipnum--
				continue
			}
			if start {
				//:id:int and :name:string
				if v == ':' {
					if len(seg) >= i+4 {
						if seg[i+1:i+4] == "int" {
							start = false
							startexp = false
							newSegment = append(newSegment, append(colonSlice, param...)...)
							skipnum = 3
							param = make([]rune, 0)
							continue
						}
					}
					if len(seg) >= i+7 {
						if seg[i+1:i+7] == "string" {
							start = false
							startexp = false
							newSegment = append(newSegment, append(colonSlice, param...)...)
							skipnum = 6
							param = make([]rune, 0)
							continue
						}
					}
				}
				// params only support a-zA-Z0-9
				if reg.MatchString(string(v)) {
					param = append(param, v)
					continue
				}
				if v != '(' {
					newSegment = append(newSegment, append(colonSlice, param...)...)
					param = make([]rune, 0)
					start = false
					startexp = false
				}
			}
			if startexp {
				if v != ')' {
					continue
				}
			}
			// Escape Sequence '\'
			if i > 0 && seg[i-1] == '\\' {
				newSegment = append(newSegment, v)
			} else if v == ':' {
				param = make([]rune, 0)
				start = true
			} else if v == '(' {
				startexp = true
				start = false
				if len(param) > 0 {
					newSegment = append(newSegment, append(colonSlice, param...)...)
					param = make([]rune, 0)
				}
			} else if v == ')' {
				startexp = false
				param = make([]rune, 0)
			} else if v == '?' {
				newSegment = append(newSegment, append([]rune{'?'}, param...)...)
			} else {
				newSegment = append(newSegment, v)
			}
		}
		if len(param) > 0 {
			newSegment = append(newSegment, append(colonSlice, param...)...)
		}
		return string(newSegment)
	}
	return seg
}

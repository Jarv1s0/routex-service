package route

import (
	"net/http"
	"routex-service/route/auth"
	"routex-service/route/coreapi"
	"routex-service/route/httphelper"
	"routex-service/route/serviceapi"
	"routex-service/route/sysapi"
	"routex-service/route/sysproxyapi"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

func router() *chi.Mux {
	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))

	r.Group(func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			httphelper.SendJSON(w, "success", "pong")
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware)
		r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			httphelper.SendJSON(w, "success", "auth success")
		})
		r.Mount("/service", serviceapi.Router())
		r.Mount("/sysproxy", sysproxyapi.Router())
		r.Mount("/core", coreapi.Router())
		r.Mount("/sys", sysapi.Router())
	})
	return r
}

package httpapi

import (
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type googleWalletSigner interface {
	CycleSlug() string
	SaveURL(passes.WebPass) (string, error)
}

func googleWalletHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		actor, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		if dependencies.GoogleWallet == nil {
			writeError(w, http.StatusServiceUnavailable, "wallet_unavailable", "Google Wallet is not available yet. Your web pass still works.")
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		// Identity, event, acceptance, confirmed RSVP and release are all checked
		// server-side. No attendee IDs or QR values are accepted from the browser.
		pass, err := service.WalletPass(request.Context(), actor, dependencies.GoogleWallet.CycleSlug())
		if err != nil {
			writePassError(w, err)
			return
		}
		saveURL, err := dependencies.GoogleWallet.SaveURL(pass)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "wallet_unavailable", "We couldn’t prepare your Google Wallet pass. Your web pass still works.")
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		writeJSON(w, http.StatusOK, struct {
			SaveURL string `json:"saveUrl"`
		}{saveURL})
	}
}

package appsdk

import (
	"log/slog"
	"sync"
)

var appState *AppState

func init() {
	slog.Info("sdk.state: app instance state init.")
	appState = NewAppState()
}

type AppState struct {
	State sync.Map
}

func NewAppState() *AppState {
	return &AppState{
		State: sync.Map{},
	}
}

func AddAppInstance(requestId string, app *App) {
	slog.Info("sdk.state: add app instance", "requestId", requestId)
	appState.State.Store(requestId, app)
}

func RemoveAppInstance(requestId string) {
	slog.Info("sdk.state: remove app instance", "requestId", requestId)
	appState.State.Delete(requestId)
}

func ShutdownApp(requestId string) bool {
	slog.Info("sdk.state: shutdown app", "requestId", requestId)
	if app, ok := appState.State.Load(requestId); ok {
		app.(*App).Shutdown()
		RemoveAppInstance(requestId)
		return true
	}
	return false
}

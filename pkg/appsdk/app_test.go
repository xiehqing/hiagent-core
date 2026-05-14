package appsdk

import (
	"charm.land/fantasy"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xiehqing/hiagent-core/internal/message"
	"os"
	"testing"
	"time"
)

func TestAppRun(t *testing.T) {
	var opts = []Option{
		WithDatabaseDriver("mysql"),
		WithDatabaseDSN("root:zorkdata.8888@tcp(192.168.12.34:3306)/crush_dev?charset=utf8mb4&parseTime=True&loc=Local"),
		WithWorkDir("C:\\projectData\\biddata\\ceshi\\bid\\test"),
		WithSkipPermissionRequests(true),
		//WithConfigScope(config.ScopeWorkspace),
		WithDebug(false),
		//WithSelectedProvider("deepseek"),
		//WithSelectedModel("deepseek-reasoner"),
	}
	app, err := New(context.Background(), opts...)
	if err != nil {
		t.Error(err)
		return
	}
	res, err := app.SubmitMessage(context.Background(), "你好", "asdasda", false)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(res.Response.Content)
}

type RunResponse struct {
	result *fantasy.AgentResult
	err    error
}

func TestNew(t *testing.T) {
	var opts = []Option{
		WithDatabaseDriver("mysql"),
		WithDatabaseDSN("root:zorkdata.8888@tcp(192.168.12.34:3306)/crush_dev?charset=utf8mb4&parseTime=True&loc=Local"),
		WithWorkDir("C:\\projectData\\biddata\\ceshi\\bid\\extract"),
		WithSkipPermissionRequests(true),
		WithDebug(false),
		WithSelectedProvider("deepseek"),
		WithSelectedModel("deepseek-reasoner"),
	}
	app, err := New(context.Background(), opts...)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer app.Shutdown()

	done := make(chan RunResponse, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sessionID := "appsdk-test-new"
	prompt := "浣犲ソ"

	go func(ctx context.Context, sessionID, prompt string) {
		result, err := app.SubmitMessage(ctx, prompt, sessionID, false)
		if err != nil {
			done <- RunResponse{
				err: fmt.Errorf("failed to start agent processing stream: %w", err),
			}
			return
		}
		done <- RunResponse{
			result: result,
		}
	}(ctx, sessionID, prompt)

	messageEvents := app.SubscribeMessage(ctx)
	streamFinished := false

	for {
		select {
		case res := <-done:
			if res.err != nil {
				if streamFinished && errors.Is(res.err, context.Canceled) {
					return
				}
				fmt.Println(res.err)
				return
			}
			fmt.Println(res.result)
			return
		case event := <-messageEvents:
			msg := event.Payload
			if msg.SessionID != sessionID || msg.Role != message.Assistant {
				continue
			}

			s, _ := json.Marshal(msg)
			fmt.Println(string(s))
			if msg.IsFinished() {
				streamFinished = true
				cancel()
			}
		case <-ctx.Done():
			if !streamFinished {
				fmt.Println("ctx done")
			}
			return
		}
	}
}

func TestApi(t *testing.T) {
	//var opts = []Option{
	//	WithDatabaseDriver("mysql"),
	//	WithDatabaseDSN("root:zorkdata.8888@tcp(192.168.12.34:3306)/crush_dev?charset=utf8mb4&parseTime=True&loc=Local"),
	//	WithWorkDir("C:\\projectData\\biddata\\ceshi\\bid\\extract"),
	//	WithSkipPermissionRequests(true),
	//	WithDebug(false),
	//	WithSelectedProvider("deepseek"),
	//	WithSelectedModel("deepseek-reasoner"),
	//}
	conn, err := handleDatabaseConnection(context.Background(), "", &DatabaseConfig{
		Driver: "mysql",
		DSN:    "root:zorkdata.8888@tcp(192.168.12.34:3306)/agent_engine?charset=utf8mb4&parseTime=True&loc=Local",
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.Close()
	service, err := NewDBService(conn)
	if err != nil {
		t.Error(err)
		return
	}
	messages, err := service.SessionMessages(context.Background(), "9ec96ef4-3e81-482f-9742-5e2cf79faeaa")
	if err != nil {
		t.Error(err)
		return
	}
	bytes, err := json.Marshal(messages)
	if err != nil {
		t.Error(err)
		return
	}
	os.WriteFile("msg.json", bytes, 0666)
	//app, err := New(context.Background(), opts...)
	//if err != nil {
	//	fmt.Println(err)
	//	return
	//}
	//defer app.Shutdown()
	//bytes, err := json.Marshal(providers)
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Log(string(bytes))
}

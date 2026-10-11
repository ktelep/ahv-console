package prism

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"ahv-console/internal/fault"
)

type taskResult struct {
	Status string `json:"status"`
	Errors []struct {
		Code  string `json:"code"`
		Group string `json:"errorGroup"`
	} `json:"errorMessages"`
	Details []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	} `json:"completionDetails"`
}

// The boolean reports whether submission may have reached PC even on failure.
func (c *Client) submitTask(ctx context.Context, path string, body any, operation string) (string, bool, error) {
	id, err := requestUUID()
	if err != nil {
		return "", false, err
	}
	uncertain := false
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return "", uncertain, ctx.Err()
		}
		var r struct {
			Data struct {
				ID string `json:"extId"`
			} `json:"data"`
		}
		status, e := c.requestOperation(ctx, "POST", path, nil, id, body, operation, &r)
		if e == nil {
			if status != 202 || !taskPattern.MatchString(r.Data.ID) {
				return "", true, fault.New(fault.Protocol, "PC returned an unexpected "+operation+" task response; outcome is unconfirmed.")
			}
			return r.Data.ID, true, nil
		}
		uncertain = uncertain || status == 0 || status >= 500 || (status >= 200 && status < 300)
		var f *fault.Error
		retryable := status == 502 || status == 503 || status == 504 || (status == 0 && errors.As(e, &f) && f.Kind == fault.Connection)
		if attempt >= 2 || !retryable {
			return "", uncertain, e
		}
		if err := wait(ctx, time.Duration(attempt+1)*c.pollInterval); err != nil {
			return "", uncertain, err
		}
	}
}

func (c *Client) pollTask(ctx context.Context, id, operation string) (taskResult, error) {
	delay := c.pollInterval
	for {
		var r struct {
			Data taskResult `json:"data"`
		}
		_, err := c.requestOperation(ctx, "GET", "/api/prism/v4.4/config/tasks/"+url.PathEscape(id), nil, "", nil, operation+" task read", &r)
		if err != nil {
			return taskResult{}, err
		}
		switch r.Data.Status {
		case "SUCCEEDED":
			return r.Data, nil
		case "FAILED", "CANCELED":
			for _, message := range r.Data.Errors {
				if message.Code == "VMM-30500" || message.Group == "VM_UNAUTHORIZED_REQUEST" {
					return r.Data, fault.New(fault.Permission, "PC denied "+operation+"; verify permission for the selected VM.")
				}
			}
			return r.Data, fault.New(fault.Task, "PC "+operation+" task failed or was canceled; inspect its prerequisites in Prism.")
		case "QUEUED", "RUNNING", "CANCELING", "SUSPENDED":
		default:
			return taskResult{}, fault.New(fault.Protocol, "PC returned an unknown "+operation+" task status.")
		}
		if err := wait(ctx, delay); err != nil {
			return taskResult{}, err
		}
		delay = min(delay*2, 2*time.Second)
	}
}

type MutationResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (c *Client) mutation(ctx context.Context, path string, body any, operation string) MutationResult {
	ctx, cancel := context.WithTimeout(ctx, c.setupTimeout)
	defer cancel()
	id, uncertain, err := c.submitTask(ctx, path, body, operation)
	var task taskResult
	if err == nil {
		task, err = c.pollTask(ctx, id, operation)
	}
	if err == nil {
		return MutationResult{"succeeded", operation + " task succeeded; refreshing observed state."}
	}
	if task.Status == "CANCELED" {
		return MutationResult{"canceled", operation + " task was canceled by PC."}
	}
	if task.Status == "FAILED" || !uncertain {
		return MutationResult{"failed", err.Error()}
	}
	return MutationResult{"unconfirmed", operation + " outcome is unconfirmed; refresh state and inspect Prism before retrying. " + err.Error()}
}

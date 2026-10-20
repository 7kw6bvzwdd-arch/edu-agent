package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type workspaceConversation struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Revision  int             `json:"revision"`
	UpdatedAt time.Time       `json:"updated_at"`
	State     json.RawMessage `json:"state,omitempty"`
}

var workspaceConversationID = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,64}$`)

func (s *Server) workspaceConversations(w http.ResponseWriter, r *http.Request, a Actor) error {
	id := r.PathValue("conversation")
	if id != "" && !workspaceConversationID.MatchString(id) {
		return fail(400, "无效对话编号")
	}
	var body struct {
		Revision int             `json:"revision"`
		State    json.RawMessage `json:"state"`
	}
	title := "备课对话"
	if r.Method == "PUT" {
		if err := decode(r, &body); err != nil {
			return err
		}
		var state struct {
			Version int `json:"version"`
			Session struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			} `json:"session"`
			Form map[string]any `json:"form"`
		}
		if body.Revision < 0 || len(body.State) > 1500000 || json.Unmarshal(body.State, &state) != nil || state.Version != 1 || state.Form == nil || len(state.Session.Messages) == 0 || len(state.Session.Messages) > 1000 {
			return fail(400, "对话记录无效或过大，请新建对话")
		}
		for _, m := range state.Session.Messages {
			if m.Role != "user" && m.Role != "assistant" {
				return fail(400, "无效消息角色")
			}
			if title == "备课对话" && m.Role == "user" && strings.TrimSpace(m.Content) != "" {
				v := []rune(strings.TrimSpace(m.Content))
				title = string(v[:min(60, len(v))])
			}
		}
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if r.Method == "GET" {
		if id != "" {
			var row workspaceConversation
			err = tx.QueryRow(r.Context(), "SELECT id,title,revision,updated_at,state FROM workspace_conversations WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", id, a.MemberID).Scan(&row.ID, &row.Title, &row.Revision, &row.UpdatedAt, &row.State)
			if err != nil {
				return err
			}
			respond(w, 200, row)
			return nil
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 || offset > 100000 {
			return fail(400, "无效历史页码")
		}
		rows, err := tx.Query(r.Context(), "SELECT id,title,revision,updated_at FROM workspace_conversations WHERE member_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC,id LIMIT 50 OFFSET $2", a.MemberID, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []workspaceConversation{}
		for rows.Next() {
			var row workspaceConversation
			if err = rows.Scan(&row.ID, &row.Title, &row.Revision, &row.UpdatedAt); err != nil {
				return err
			}
			list = append(list, row)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		respond(w, 200, list)
		return nil
	}
	if r.Method == "DELETE" {
		if id != "" {
			// A failed first save may leave only a client ID. Fence that ID as well.
			if _, err = tx.Exec(r.Context(), "INSERT INTO workspace_conversations(id,tenant_id,member_id,title,state,revision) VALUES($1,$2,$3,'','{}',0) ON CONFLICT DO NOTHING", id, a.TenantID, a.MemberID); err != nil {
				return err
			}
		}
		query := "UPDATE workspace_conversations SET state='{}',title='',deleted_at=now(),revision=revision+1 WHERE member_id=$1 AND deleted_at IS NULL"
		args := []any{a.MemberID}
		if id != "" {
			query += " AND id=$2"
			args = append(args, id)
		}
		result, err := tx.Exec(r.Context(), query, args...)
		if err != nil {
			return err
		}
		if id != "" && result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		if err = tx.Commit(r.Context()); err != nil {
			return err
		}
		respond(w, 200, map[string]bool{"ok": true})
		return nil
	}
	// Retain empty deletion markers so a late autosave cannot resurrect cleared data.
	if _, err = tx.Exec(r.Context(), "INSERT INTO workspace_conversations(id,tenant_id,member_id,title,state,revision) VALUES($1,$2,$3,'','{}',0) ON CONFLICT DO NOTHING", id, a.TenantID, a.MemberID); err != nil {
		return err
	}
	var revision int
	var deleted bool
	var old json.RawMessage
	err = tx.QueryRow(r.Context(), "SELECT revision,deleted_at IS NOT NULL,state FROM workspace_conversations WHERE id=$1 AND member_id=$2 FOR UPDATE", id, a.MemberID).Scan(&revision, &deleted, &old)
	if err != nil {
		return err
	}
	if deleted {
		return fail(409, "对话已清除，不再保存迟到的回复")
	}
	if revision != body.Revision {
		var x, y any
		json.Unmarshal(old, &x)
		json.Unmarshal(body.State, &y)
		if revision != body.Revision+1 || !bytes.Equal(asJSON(x), asJSON(y)) {
			return fail(409, "对话已在其他页面更新，请重新打开历史记录")
		}
	} else {
		revision++
		if _, err = tx.Exec(r.Context(), "UPDATE workspace_conversations SET title=$1,state=$2,revision=$3,updated_at=now() WHERE id=$4 AND member_id=$5", title, body.State, revision, id, a.MemberID); err != nil {
			return err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]int{"revision": revision})
	return nil
}

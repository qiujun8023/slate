package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"slate/internal/api"
	"slate/internal/store"
	"slate/internal/store/db"
)

func (a *API) ownedGroup(ctx context.Context, id, uid string) (db.Group, error) {
	g, err := a.Store.Q.GetGroup(ctx, id)
	if store.Missing(err) || (err == nil && g.OwnerID != uid) {
		return g, notFound("内容组")
	}
	return g, err
}

func (a *API) groupView(ctx context.Context, uid, id string) (api.Group, error) {
	rows, err := a.Store.Q.ListGroups(ctx, uid)
	if err != nil {
		return api.Group{}, err
	}
	for _, g := range rows {
		if g.ID == id {
			return api.Group{ID: g.ID, Name: g.Name, SortOrder: g.SortOrder, ContentCount: int(g.ContentCount), TotalBytes: g.TotalBytes}, nil
		}
	}
	return api.Group{}, notFound("内容组")
}

func groupName(req api.GroupRequest) (string, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 64 {
		return "", badRequest("内容组名称须为 1-64 个字符")
	}
	return name, nil
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.Store.Q.ListGroups(r.Context(), userID(r))
	if err != nil {
		return err
	}
	out := make([]api.Group, len(rows))
	for i, g := range rows {
		out[i] = api.Group{ID: g.ID, Name: g.Name, SortOrder: g.SortOrder, ContentCount: int(g.ContentCount), TotalBytes: g.TotalBytes}
	}
	return ok(w, out)
}

func (a *API) getGroup(w http.ResponseWriter, r *http.Request) error {
	g, err := a.groupView(r.Context(), userID(r), r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, g)
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) error {
	var req api.GroupRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	name, err := groupName(req)
	if err != nil {
		return err
	}
	uid := userID(r)
	var g db.Group
	err = a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if _, err := q.LockUser(r.Context(), uid); err != nil {
			return err
		}
		sort, err := q.NextGroupSortOrder(r.Context(), uid)
		if err != nil {
			return err
		}
		if g, err = q.CreateGroup(r.Context(), db.CreateGroupParams{ID: newID(), OwnerID: uid, Name: name, SortOrder: sort}); err != nil {
			return err
		}
		// The first group is assigned to bound devices that have none, so a new user never
		// has to assign it by hand; later groups leave deliberately empty devices alone.
		if n, err := q.CountGroups(r.Context(), uid); err != nil || n != 1 {
			return err
		}
		bound, err := q.AssignGroupToUngroupedDevices(r.Context(), db.AssignGroupToUngroupedDevicesParams{OwnerID: &uid, GroupID: &g.ID})
		if bound > 0 {
			slog.Info("first group assigned to devices", "groupId", g.ID, "devices", bound)
		}
		return err
	})
	if err != nil {
		return err
	}
	return created(w, api.Group{ID: g.ID, Name: g.Name, SortOrder: g.SortOrder})
}

func (a *API) renameGroup(w http.ResponseWriter, r *http.Request) error {
	var req api.GroupRequest
	if err := decodeJSON(r, 16<<10, &req); err != nil {
		return err
	}
	name, err := groupName(req)
	if err != nil {
		return err
	}
	g, err := a.ownedGroup(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if err := a.Store.Q.RenameGroup(r.Context(), db.RenameGroupParams{ID: g.ID, Name: name}); err != nil {
		return err
	}
	return a.getGroup(w, r)
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) error {
	g, err := a.ownedGroup(r.Context(), r.PathValue("id"), userID(r))
	if err != nil {
		return err
	}
	if err := a.Store.Q.DeleteGroup(r.Context(), g.ID); err != nil {
		return err
	}
	return ok(w, nil)
}

func (a *API) reorderGroups(w http.ResponseWriter, r *http.Request) error {
	var req api.OrderRequest
	if err := decodeJSON(r, 64<<10, &req); err != nil {
		return err
	}
	uid := userID(r)
	err := a.Store.Tx(r.Context(), func(q *db.Queries) error {
		if _, err := q.LockUser(r.Context(), uid); err != nil {
			return err
		}
		ids, err := q.ListGroupIDs(r.Context(), uid)
		if err != nil {
			return err
		}
		if err := validateOrder(ids, req.Order, "内容组"); err != nil {
			return err
		}
		return q.SetGroupSortOrders(r.Context(), db.SetGroupSortOrdersParams{Ids: req.Order, OwnerID: uid})
	})
	if err != nil {
		return err
	}
	return ok(w, nil)
}

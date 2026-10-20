package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTimelinePreservesFiveAndSixEventsOnOnePage(t *testing.T) {
	for _, count := range []int{5, 6} {
		items := []FigureItem{{Label: "鸦片战争1840—1842"}, {Label: "第二次鸦片战争1856—1860"}, {Label: "甲午中日战争1894—1895"}, {Label: "戊戌变法1898"}, {Label: "辛亥革命1911"}, {Label: "中华民国成立1912"}}[:count]
		sl := Slide{Figure: &TeachingFigure{Kind: "timeline", Title: "近代史主干事件顺序", Items: items}}
		if err := validateTeachingMedia(sl); err != nil {
			t.Fatalf("%d-event timeline rejected: %v", count, err)
		}
		d := arrangeCoursewareFigures(Deck{Slides: []Slide{sl}})
		if len(d.Slides) != 1 || len(d.Slides[0].Figure.Items) != count {
			t.Fatal("timeline unexpectedly split or lost events")
		}
	}
}

func TestTimelineSixNodeBoundaries(t *testing.T) {
	for _, kind := range []string{"timeline", "flow", "route", "interval"} {
		items := make([]FigureItem, 7)
		for i := range items {
			items[i].Label = "事件"
		}
		if validateTeachingMedia(Slide{Figure: &TeachingFigure{Kind: kind, Items: items}}) == nil {
			t.Fatal("oversize figure accepted", kind)
		}
		if kind != "timeline" && validateTeachingMedia(Slide{Figure: &TeachingFigure{Kind: kind, Items: items[:5]}}) == nil {
			t.Fatal("non-timeline limit changed", kind)
		}
	}
}

func TestHistoryTimelineDraftAndReviewIsolationHTTP(t *testing.T) {
	s := worksServer(t)
	token, other := worksLogin(t, s, "teacher"), worksLogin(t, s, "other")
	id := worksFixture(t, s, token, "courseware", "awaiting_preview", "近代史", TaskInput{PreviewFirst: true})
	route := "/api/tasks/" + id
	var initial struct{ Revision int }
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &initial)
	d := sampleDeck(TaskInput{Topic: "近代史", Pages: 8, Theme: "ocean"})
	d.Slides = d.Slides[:3]
	assignSlideIDs(&d)
	d.Slides[0] = Slide{ID: d.Slides[0].ID, Title: "一条时间轴看近代史主线", Bullets: []string{"五个主干事件串成一条主线"}, Notes: "测试课件：按事件发生的先后顺序讲解，时间轴不按年份比例绘制。",
		Figure: &TeachingFigure{Kind: "timeline", Title: "近代史主干事件顺序", Items: []FigureItem{{Label: "鸦片战争1840—1842"}, {Label: "第二次鸦片战争1856—1860"}, {Label: "甲午中日战争1894—1895"}, {Label: "戊戌变法1898"}, {Label: "辛亥革命1911", Detail: "1912年中华民国成立"}}}, Picture: teachingTestPicture(t)}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	body := map[string]any{"revision": initial.Revision, "result": d}
	worksCall(t, s, "POST", route+"/draft", other, body, 404)
	worksCall(t, s, "POST", route+"/draft", token, body, 200)
	var saved struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &saved)
	if len(saved.Result.Slides) != 3 || len(saved.Result.Slides[0].Figure.Items) != 5 {
		t.Fatal("planned page or event lost")
	}
	pptx := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(pptx) < 1000 || string(pptx[:2]) != "PK" {
		t.Fatal("real PPTX missing")
	}
	review := map[string]any{"revision": saved.Revision, "hash": slideContentHash(saved.Result, saved.Result.Slides[0]), "approved": true}
	path := route + "/slides/" + saved.Result.Slides[0].ID + "/review"
	worksCall(t, s, "POST", path, other, review, 404)
	worksCall(t, s, "POST", path, token, review, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET status='failed' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", path, token, review, 409)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("draft/review changed billing")
	}
}

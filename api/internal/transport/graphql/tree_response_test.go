package graphql_test

// Kiểu response cho cây roadmap 5 tầng, dùng chung bởi mọi test `graph/client`.
//
// Tách ra file riêng + có `json` tag TƯỜNG MINH vì `client.Client` khớp field Go
// với key GraphQL theo `json` tag (không phải theo tên field): struct ẩn danh viết
// inline trong test dễ quên tag, và lỗi biểu hiện là panic "has invalid keys" chứ
// không phải compile error.

type treePathResp struct {
	ID       string           `json:"id"`
	GUID     string           `json:"guid"`
	Slug     string           `json:"slug"`
	Title    string           `json:"title"`
	Progress treeProgressResp `json:"progress"`
	Stages   []treeStageResp  `json:"stages"`
}

type treeProgressResp struct {
	Percent     int `json:"percent"`
	TopicsTotal int `json:"topicsTotal"`
	TopicsDone  int `json:"topicsDone"`
}

type treeStageResp struct {
	ID         string              `json:"id"`
	GUID       string              `json:"guid"`
	Title      string              `json:"title"`
	Position   int                 `json:"position"`
	Terrain    string              `json:"terrain"`
	Direction  string              `json:"direction"`
	Deck       *treeDeckResp       `json:"deck"`
	Milestones []treeMilestoneResp `json:"milestones"`
	Topics     []treeTopicResp     `json:"topics"`
}

type treeDeckResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}

type treeMilestoneResp struct {
	ID   string `json:"id"`
	GUID string `json:"guid"`
	Text string `json:"text"`
}

type treeTopicResp struct {
	ID         string             `json:"id"`
	GUID       string             `json:"guid"`
	Title      string             `json:"title"`
	IsOptional bool               `json:"isOptional"`
	MapPinned  bool               `json:"mapPinned"`
	Level      string             `json:"level"`
	Point      treePointResp      `json:"point"`
	Resources  []treeResourceResp `json:"resources"`
}

type treePointResp struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type treeResourceResp struct {
	ID    string `json:"id"`
	GUID  string `json:"guid"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
)

// ── Request / Response 구조체 ──

type Request struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
}

type Response struct {
	ParentPID int `json:"parent_pid"`
	ChildPID  int `json:"child_pid"`
}

// ── 핵심 핸들러 ──

func netnsHandler(w http.ResponseWriter, r *http.Request) {

	// 1) JSON body 파싱
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// 2) 자식 프로세스 준비
	cmd := exec.Command(req.Path, req.Args...)

	// 3) CLONE_NEWNET으로 새 Network Namespace 부여
	//    이 flag 덕분에 자식만 격리되고, 부모(HTTP 서버)는 영향 없음
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNET,
	}

	// 4) 자식 프로세스 시작 (non-blocking)
	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf(`{"error":"failed to start: %s"}`, err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	// 5) zombie 방지: 백그라운드에서 자식 종료를 수거
	go func() {
		_ = cmd.Wait()
	}()

	// 6) 응답 구성
	resp := Response{
		ParentPID: os.Getpid(),     // HTTP 서버 자신의 PID
		ChildPID:  cmd.Process.Pid, // 방금 띄운 자식의 PID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func main() {
	http.HandleFunc("/unshare/netns", netnsHandler)

	fmt.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
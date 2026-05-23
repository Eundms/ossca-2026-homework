package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"syscall"
)

type RequestPayload struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
}

type ResponsePayload struct {
	ParentPID int `json:"parent_pid"`
	ChildPID  int `json:"child_pid"`
}

func netnsHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 자식 프로세스로 실행할 명령어 준비
	// exec.Command(실행파일경로, 인자들...)
	cmd := exec.Command(req.Path, req.Args...)

	// 자식 프로세스가 새로운 Network Namespace에서 실행되도록 설정
	// Unshareflags에 CLONE_NEWNET을 지정하면, 자식 프로세스가 exec 전에
	// unshare(CLONE_NEWNET) 시스템 콜을 호출하여 독립된 네트워크 공간을 갖게 됨.
	// 부모(HTTP 서버)의 네트워크 네임스페이스는 변경되지 않는다.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNET,
	}

	// 자식 프로세스 시작 (완료를 기다리지 않음)
	if err := cmd.Start(); err != nil {
		http.Error(w, "Failed to start process: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 좀비 프로세스 방지: 별도 고루틴에서 자식 프로세스 종료를 대기(wait)
	// 부모가 wait하지 않으면 자식이 종료되어도 좀비 상태로 남게 .
	// 고루틴으로 처리하여 HTTP 응답을 즉시 반환하면서도 좀비를 방지.
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("child process %d exited with error: %v", cmd.Process.Pid, err)
		}
	}()

	// 응답 생성: 부모 PID와 자식 PID를 JSON으로 반환
	resp := ResponsePayload{
		ParentPID: os.Getpid(),
		ChildPID:  cmd.Process.Pid,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	http.HandleFunc("/unshare/netns", netnsHandler)

	log.Println("Server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

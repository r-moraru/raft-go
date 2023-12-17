package main

import (
	"context"
	"log"
	"net"
	"sync"

	pb "github.com/r-moraru/raft-go/proto"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedRaftServer
	lock sync.RWMutex

	// Server persistent state
	CurrentTerm uint64
	VotedFor    *uint64
	Log         []*pb.LogEntry

	// Server volatile state
	CommitIndex uint64
	lastApplied uint64

	// Leader volatile state
	NextIndex  []uint64
	MatchIndex []uint64
}

func (s *server) AppendEntries(context context.Context, request *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	response := new(pb.AppendEntriesResponse)
	response.Term = new(uint64)
	*response.Term = s.CurrentTerm
	response.Success = new(bool)
	*response.Success = false

	if s.CurrentTerm > request.GetTerm() {
		return response, nil
	}
	if len(s.Log) <= int(request.GetPrevLogIndex()) || s.Log[request.GetPrevLogIndex()].GetTerm() != request.GetPrevLogTerm() {
		return response, nil
	}
	for _, entry := range request.Entries {
		if len(s.Log) <= int(entry.GetIndex()) && s.Log[entry.GetIndex()].GetTerm() != entry.GetTerm() {
			s.Log = s.Log[:entry.GetIndex()]
			break
		}
	}
	for _, entry := range request.Entries {
		if len(s.Log) <= int(entry.GetIndex()) {
			continue
		}
		s.Log = append(s.Log, entry)
	}

	if request.GetLeaderCommit() > s.CommitIndex {
		s.CommitIndex = min(request.GetLeaderCommit(), uint64(len(s.Log)))
	}
	return response, nil
}

func (s *server) RequestVote(context context.Context, request *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	response := new(pb.RequestVoteResponse)
	response.Term = new(uint64)
	*response.Term = s.CurrentTerm
	response.VoteGranted = new(bool)
	*response.VoteGranted = false

	if request.GetTerm() >= s.CurrentTerm && s.VotedFor != nil &&
		(request.GetLastLogIndex() < uint64(len(s.Log)) ||
			request.GetLastLogTerm() < s.CurrentTerm) {
		return response, nil
	}

	*response.VoteGranted = true
	return response, nil
}

func main() {
	lis, err := net.Listen("tcp", "localhost:5001")
	if err != nil {
		log.Fatalf("failed to listed: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterRaftServer(s, &server{})

	serverClosed := make(chan bool)
	go func() {
		log.Printf("server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	done := <-serverClosed
	log.Printf("%v", done)

	// TODO: create client-facing server

	for {
		switch {
		case <-serverClosed:
			log.Printf("Server closed.")
			return
			// TODO:
			// 1. election timeout -> become candidate
			// 		increment currentTerm
			//
			// 2. apply log[lastApplied] if commitIndex > lastApplied
			// 3. Listen for commands from clients
		}
	}

	// TODO: candidate loop
	// wait for replies or election timeout
}

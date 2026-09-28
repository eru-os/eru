package a2a

import (
	"errors"
	"sync"
)

type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*Task)}
}

func (s *TaskStore) Save(task *Task) {
	s.mu.Lock()
	s.tasks[task.Id] = task
	s.mu.Unlock()
}

func (s *TaskStore) Get(id string) (*Task, error) {
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("task not found: " + id)
	}
	return task, nil
}

func (s *TaskStore) UpdateStatus(id string, status TaskStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found: " + id)
	}
	task.Status = status
	return nil
}

func (s *TaskStore) Cancel(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("task not found: " + id)
	}
	if task.Status.State == TaskStateCompleted || task.Status.State == TaskStateFailed {
		return nil, errors.New("task already in terminal state")
	}
	task.Status.State = TaskStateCanceled
	return task, nil
}

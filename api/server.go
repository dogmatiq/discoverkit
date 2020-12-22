package api

import (
	"sync"

	"github.com/dogmatiq/configkit"
	"github.com/dogmatiq/interopspec/discoverspec"
)

// Server is an implementation of discoverspec.DiscoverAPIServer.
type Server struct {
	m sync.Mutex

	// available is the set of applications that are currently available.
	//
	// The key is the application's identity key.
	//
	// The map value itself is immutable. When a change to the available
	// applications is made, the map is cloned and the changes applied to the
	// clone. Finally, s.available is updated to refer to the clone.
	//
	// This allows many goroutines to read from any given "version" of the map
	// without holding any locks.
	available map[string]*discoverspec.Identity

	// changed is a "broadcast" channel that is closed to signal that the set of
	// available applications has been replaced with a new "version".
	changed chan struct{}
}

var _ discoverspec.DiscoverAPIServer = (*Server)(nil)

// Available marks the given application as available.
func (s *Server) Available(app configkit.Identity) {
	s.update(app, true)
}

// Unavailable marks the given application as unavailable.
func (s *Server) Unavailable(app configkit.Identity) {
	s.update(app, false)
}

// update the availability of the given app.
func (s *Server) update(app configkit.Identity, available bool) {
	if err := app.Validate(); err != nil {
		panic(err)
	}

	s.m.Lock()
	defer s.m.Unlock()

	if _, ok := s.available[app.Key]; ok == available {
		// The desired availability is the same as the app's current
		// availability, do nothing.
		return
	}

	// Create a clone of s.available. This avoids any data races with other
	// goroutines reading the map currently references by s.available.
	next := map[string]*discoverspec.Identity{}
	for k, ident := range s.available {
		next[k] = ident
	}

	// Update the clone to reflect the change.
	if available {
		next[app.Key] = &discoverspec.Identity{
			Name: app.Name,
			Key:  app.Key,
		}
	} else {
		delete(next, app.Key)
	}

	// Replace s.available with the clone.
	s.available = next

	// Notify the watchers that a change has been made.
	if s.changed != nil {
		close(s.changed)
		s.changed = nil
	}
}

// capture returns the current set of available applications, and a channel that
// is closed if the set of available applications changes.
func (s *Server) capture() (map[string]*discoverspec.Identity, <-chan struct{}) {
	s.m.Lock()
	defer s.m.Unlock()

	if s.changed != nil {
		s.changed = make(chan struct{})
	}

	return s.available, s.changed
}

// Watch starts watching the server for updates to the availability of Dogma
// applications.
func (s *Server) Watch(
	_ *discoverspec.WatchRequest,
	stream discoverspec.DiscoverAPI_WatchServer,
) error {
	// Keep a reference to the previous map of available applications. This is
	// used to compute a "diff" when the available applications is updated.
	//
	// This approach is taken as it allows changes to the available applications
	// to be applied in s.Available() and s.Unavailable() without waiting for
	// each individual Watch() consumer to receive its responses.
	var prev map[string]*discoverspec.Identity

	for {
		// Read the current list of available applications.
		next, changed := s.capture()

		// Send an "available" response for each application that is in "next",
		// but not in "prev".
		if err := s.diff(stream, true, next, prev); err != nil {
			return err
		}

		// Send an "unavailable" response for each application that is in
		// "prev", but not in "next".
		if err := s.diff(stream, false, prev, next); err != nil {
			return err
		}

		select {
		case <-stream.Context().Done():
			// The client has disconnected, or the server has been stopped.
			return stream.Context().Err()
		case <-changed:
			// The list of available applications has changed.
			prev = next
		}
	}
}

// diff sends a WatchResponse for each application that is present in lhs but
// not present in rhs.
func (s *Server) diff(
	stream discoverspec.DiscoverAPI_WatchServer,
	available bool,
	lhs, rhs map[string]*discoverspec.Identity,
) error {
	for k, app := range lhs {
		if _, ok := rhs[k]; ok {
			continue
		}

		res := &discoverspec.WatchResponse{
			Application: app,
			Available:   available,
		}

		if err := stream.Send(res); err != nil {
			return err
		}
	}

	return nil
}

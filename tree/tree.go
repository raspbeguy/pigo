// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package tree builds a hierarchical view of the page collection and exposes
// the pages() query used by templates.
package tree

import (
	"strings"

	"github.com/raspbeguy/pigo/content"
)

// Node is one level in the page tree. Children is keyed by the final path
// segment; Page is the page at this exact path (may be nil for directories
// without an index).
type Node struct {
	ID       string
	Page     *content.Page
	Children map[string]*Node
}

// Build constructs the tree from a flat, sorted page slice.
//
// A page whose final id segment is "index" represents the folder itself and
// attaches to the parent node's Page (mirroring how Pico treats folder
// indexes). For example "sub/index" becomes root.Children["sub"].Page, not a
// child named "index".
func Build(pages []*content.Page) *Node {
	root := &Node{Children: map[string]*Node{}}
	for _, p := range pages {
		segs := strings.Split(p.ID, "/")
		// Handle "index" as the folder's own page.
		lastIsIndex := segs[len(segs)-1] == "index"
		if lastIsIndex {
			segs = segs[:len(segs)-1]
		}
		node := root
		var idBuilder strings.Builder
		for i, seg := range segs {
			if idBuilder.Len() > 0 {
				idBuilder.WriteByte('/')
			}
			idBuilder.WriteString(seg)
			child, ok := node.Children[seg]
			if !ok {
				child = &Node{ID: idBuilder.String(), Children: map[string]*Node{}}
				node.Children[seg] = child
			}
			node = child
			_ = i
		}
		node.Page = p
	}
	return root
}

// AsMap converts the tree into a map[string]any for template rendering,
// matching the shape {{ page_tree.child.subchild._page }}.
func (n *Node) AsMap() map[string]any {
	out := map[string]any{}
	if n.Page != nil {
		out["_page"] = n.Page.AsMap()
	}
	for name, child := range n.Children {
		out[name] = child.AsMap()
	}
	return out
}

// Query implements Pico's pages() Twig function:
//
//	start        — starting path (empty = tree root)
//	depth        — max depth below start to include (0 = unlimited)
//	depthOffset  — minimum depth below start
//	offset       — depth offset applied to start itself (1 = below start,
//	               0 = include start)
//
// See Pico's PicoTwigExtension.php pages() function for exact semantics.
func (n *Node) Query(start string, depth, depthOffset, offset int) []*content.Page {
	startNode := n.find(start)
	if startNode == nil {
		return nil
	}
	var out []*content.Page
	startNode.collect(&out, 0, depth, depthOffset, offset)
	return out
}

func (n *Node) find(start string) *Node {
	if start == "" {
		return n
	}
	node := n
	for _, seg := range strings.Split(start, "/") {
		child, ok := node.Children[seg]
		if !ok {
			return nil
		}
		node = child
	}
	return node
}

func (n *Node) collect(out *[]*content.Page, curDepth, maxDepth, minDepth, offset int) {
	effective := curDepth - offset
	if effective >= minDepth && (maxDepth == 0 || effective <= maxDepth) {
		if n.Page != nil && !n.Page.Hidden {
			*out = append(*out, n.Page)
		}
	}
	if maxDepth != 0 && curDepth-offset >= maxDepth {
		return
	}
	for _, child := range n.Children {
		child.collect(out, curDepth+1, maxDepth, minDepth, offset)
	}
}

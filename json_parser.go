package jsonextract

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type PathNode struct {
	Name         string
	Key          []byte // the json key value to match for this node
	Children     []*PathNode
	Filter       *PathFilter
	ArrayIndex   int // -1 means wildcard (all)
	AsArray      bool
	IsTerminal   bool // true if this node is a terminal node in the path
	NumTerminals int
}

type PathResultWatcher struct {
	Name     string
	Children map[string]*PathResultWatcher
	Complete bool
}

func (n *PathNode) String() string {
	return "PathNode{" +
		"Name: " + n.Name +
		", Key: " + string(n.Key) +
		", Filter: " + func() string {
		if n.Filter != nil {
			return n.Filter.Key + "=" + n.Filter.Value
		}
		return ""
	}() +
		", ArrayIndex: " + strconv.Itoa(n.ArrayIndex) +
		", AsArray: " + strconv.FormatBool(n.AsArray) +
		"}"
}

type PathFilter struct {
	Key   string
	Value string
}

type Extractor struct {
	RawData            []byte
	Root               *PathNode
	Results            map[string][]string
	Scanner            *Scanner
	ResultWatcher      *PathResultWatcher
	ExtractionComplete bool
}

func CompilePaths(paths map[string]string) *PathNode {
	root := &PathNode{}
	terminals := 0
	for name, query := range paths {
		segments := strings.Split(query, ".")
		current := root
		for _, segment := range segments {
			child, found := current.FindChildByName(segment)
			if !found {
				child = &PathNode{Name: segment}
				child.Key = []byte(segment)
				current.Children = append(current.Children, child)
			}

			if strings.Contains(segment, "[") {
				child.AsArray = true

				parts := strings.Split(segment, "[")
				segment = parts[0]
				child.Key = []byte(segment)

				index := strings.TrimSuffix(parts[1], "]")

				if index == "*" {
					child.ArrayIndex = -1 // wildcard
				} else if strings.HasPrefix(index, "?") {
					filter_parts := strings.SplitN(index[1:], "=", 2)
					if len(filter_parts) == 2 {
						child.Filter = &PathFilter{
							Key:   filter_parts[0],
							Value: filter_parts[1],
						}
					}
				} else {
					var err error
					if child.ArrayIndex, err = strconv.Atoi(index); err != nil {
						child.ArrayIndex = -1 // treat as wildcard if parsing fails
					}
				}
			}

			current = child
		}
		current.Name = name
		current.IsTerminal = true
		terminals++
	}
	root.NumTerminals = terminals
	return root
}

func NewPathResultWatcher(node *PathNode) *PathResultWatcher {
	watcher := &PathResultWatcher{
		Name: node.Name,
	}
	watcher.Children = make(map[string]*PathResultWatcher)
	for _, child := range node.Children {
		watcher.Children[child.Name] = NewPathResultWatcher(child)
	}
	return watcher
}

func (r *PathResultWatcher) AllComplete() bool {
	if r.Complete {
		return true
	}
	for _, child := range r.Children {
		if !child.AllComplete() {
			return false
		}
	}
	return len(r.Children) > 0
}

func NewExtractor(rawData []byte, root *PathNode) *Extractor {
	return &Extractor{
		RawData:       rawData,
		Root:          root,
		Results:       make(map[string][]string),
		Scanner:       NewScanner(&rawData),
		ResultWatcher: NewPathResultWatcher(root),
	}
}

func (e *Extractor) Extract() error {
	tok, _ := e.Scanner.Token()
	switch tok {
	case StartObject:
		if err := e.ExtractObject(e.Root, e.ResultWatcher); err != nil {
			return err
		}
	case StartArray:
		if err := e.ExtractArray(e.Root, e.ResultWatcher); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected token %s at start of JSON", tok)
	}
	return nil
}

func (node *PathNode) FindChild(key []byte) *PathNode {
	for _, child := range node.Children {
		if bytes.Equal(child.Key, key) {
			return child
		}
	}
	return nil
}

func (p *PathNode) FindChildByName(name string) (*PathNode, bool) {
	for _, child := range p.Children {
		if child.Name == name {
			return child, true
		}
	}
	return nil, false
}

func (e *Extractor) AllResultsReturned() bool {
	for _, r := range e.ResultWatcher.Children {
		if !r.AllComplete() {
			return false
		}
	}
	return true
}

func (e *Extractor) ExtractObject(node *PathNode, resultNode *PathResultWatcher) error {
	for e.Scanner.More() {
		key, err := e.Scanner.ExpectString()
		if err != nil {
			return err
		}

		childNode := node.FindChild(key)
		if childNode == nil {
			e.Scanner.SkipValue()
			continue
		}

		tok, val := e.Scanner.Token()
		switch tok {
		case StartObject:
			if err := e.ExtractObject(childNode, resultNode.Children[childNode.Name]); err != nil {
				return err
			}
		case StartArray:
			if err := e.ExtractArray(childNode, resultNode.Children[childNode.Name]); err != nil {
				return err
			}
		default:
			if childNode.IsTerminal {
				e.AddResult(childNode, resultNode.Children[childNode.Name], false, string(val))
			} else {
				e.Scanner.SkipValue() // skip value for non-object/array tokens
			}
		}

		if e.ExtractionComplete {
			return nil
		}
	}
	if err := e.Scanner.ExpectEndObject(); err != nil {
		return err
	}

	return nil
}

func (e *Extractor) AddResult(node *PathNode, resultNode *PathResultWatcher, wildcardEnd bool, value string) {
	e.Results[node.Name] = append(e.Results[node.Name], value)
	if node.AsArray {
		if wildcardEnd {
			resultNode.Complete = true
		}
	} else {
		resultNode.Complete = true
	}
	if e.AllResultsReturned() {
		e.ExtractionComplete = true
	}
}

func (e *Extractor) EndArray(node *PathNode, resultNode *PathResultWatcher) {
	resultNode.Complete = true
	if e.AllResultsReturned() {
		e.ExtractionComplete = true
	}
}

func (e *Extractor) ExtractArray(node *PathNode, resultNode *PathResultWatcher) error {
	idx := 0
	for e.Scanner.More() {
		if node.Filter == nil && node.ArrayIndex != -1 && node.ArrayIndex != idx {
			e.Scanner.SkipValue() // skip this item if index doesn't match
			idx++
			continue
		}

		tok, val := e.Scanner.Token()
		switch tok {
		case StartObject:
			if err := e.ExtractObject(node, resultNode); err != nil {
				return err
			}
		case StartArray:
			if err := e.ExtractArray(node, resultNode); err != nil {
				return err
			}
		default:
			if node.IsTerminal {
				e.AddResult(node, resultNode, node.ArrayIndex != -1, string(val))
			}
			e.Scanner.SkipValue() // skip value for non-object/array tokens
		}

		if e.ExtractionComplete {
			return nil
		}

		idx++
	}
	e.EndArray(node, resultNode)

	if err := e.Scanner.ExpectEndArray(); err != nil {
		return err
	}

	return nil
}

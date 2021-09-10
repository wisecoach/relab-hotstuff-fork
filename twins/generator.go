package twins

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
)

type leaderPartitions struct {
	leader     uint8
	partitions []uint8
}

type Generator struct {
	rounds            uint8
	indices           []int
	partitions        int
	leadersPartitions []leaderPartitions

	replicas []hotstuff.ID
	nodes    []NodeID
}

func NewGenerator(replicas, twins, partitions, rounds uint8) *Generator {
	pg := newPartGen(replicas+twins, partitions)

	g := &Generator{
		rounds:     rounds,
		partitions: int(partitions),
		indices:    make([]int, rounds),
		replicas:   make([]hotstuff.ID, 0, replicas),
		nodes:      make([]NodeID, 0, replicas+twins),
	}

	for p := pg.nextPartitions(); p != nil; p = pg.nextPartitions() {
		for id := uint8(0); id < replicas; id++ {
			g.leadersPartitions = append(g.leadersPartitions, leaderPartitions{
				leader:     id,
				partitions: p,
			})
		}
	}

	replicaID := hotstuff.ID(1)
	networkID := uint32(1)
	remainingTwins := twins

	for i := 0; i < int(replicas); i++ {
		g.replicas = append(g.replicas, replicaID)
		g.nodes = append(g.nodes, NodeID{
			ReplicaID: replicaID,
			NetworkID: networkID,
		})
		networkID++
		if remainingTwins > 0 {
			g.nodes = append(g.nodes, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			networkID++
		}
		replicaID++
	}

	return g
}

func (g *Generator) NextScenario() Scenario {
	// This is basically computing the cartesian product of leadersPartitions with itself "round" times.
	p := make([]leaderPartitions, g.rounds)
	for i, ii := range g.indices {
		p[i] = g.leadersPartitions[ii]
	}
	for i := g.rounds - 1; i >= 0; i-- {
		g.indices[i]++
		if g.indices[i] < len(g.leadersPartitions) {
			break
		}
		g.indices[i] = 0
		if i <= 0 {
			g.indices = g.indices[0:0]
		}
	}
	s := Scenario{
		Replicas: g.replicas,
		Nodes:    g.nodes,
		Rounds:   int(g.rounds),
		ConsensusCtor: func() consensus.Consensus {
			// TODO: make this configurable
			return consensus.New(chainedhotstuff.New())
		},
		ViewTimeout: 10,
	}
	for _, partition := range p {
		s.Leaders = append(s.Leaders, g.replicas[partition.leader])
		partitions := make([]NodeSet, g.partitions)
		for i, partitionNo := range partition.partitions {
			if partitions[partitionNo] == nil {
				partitions[partitionNo] = make(NodeSet)
			}
			partitions[partitionNo].Add(g.nodes[i])
		}
		s.Partitions = append(s.Partitions, partitions)
	}
	return s
}

// partGen generates partitions of n nodes.
// It will not generate more than k partitions at once.
//
// algorithm based on:
// https://stackoverflow.com/questions/30893292/generate-all-partitions-of-a-set
type partGen struct {
	p []uint8
	m []uint8
	k uint8
}

func newPartGen(n, k uint8) partGen {
	return partGen{
		p: make([]uint8, n),
		m: make([]uint8, n), // m_0 = 0; m_i = max(m_(i-1), p_(i-1))
		k: k,
	}
}

func (g *partGen) nextPartitions() (partitionNumbers []uint8) {
	found := false
	// calculate next state
	for i := len(g.p) - 1; i >= 0; i-- {
		if g.p[i] < g.k-1 && g.p[i] <= g.m[i] {
			g.p[i]++
			if i < len(g.p)-1 {
				g.m[i+1] = max(g.m[i], g.p[i])
			}
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	// copy and return new state
	partitionNumbers = make([]uint8, len(g.p))
	copy(partitionNumbers, g.p)
	return partitionNumbers
}

func max(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}
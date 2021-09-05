package twins

import (
	"context"
	"strconv"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/synchronizer"
)

type Scenario struct {
	Replicas      []hotstuff.ID
	Leaders       []hotstuff.ID
	Nodes         []NodeID
	Partitions    [][]NodeSet
	Rounds        int
	Byzantine     int
	ConsensusCtor func() consensus.Consensus
	ViewTimeout   float64
}

func ExecuteScenario(scenario Scenario) (safe bool, commits int, err error) {
	network := NewNetwork(scenario.Partitions)

	ctx, cancel := context.WithCancel(context.Background())

	err = createNodes(scenario, network)
	if err != nil {
		cancel()
		return false, 0, err
	}

	network.StartNodes(ctx)
	network.WaitUntilHung()
	cancel()
	network.WaitUntilDone()

	// check if the majority of replicas have committed the same blocks
	// TODO
	safe, commits = checkCommits(network)

	return safe, commits, nil
}

func createNodes(scenario Scenario, network *Network) error {
	cg := &commandGenerator{}
	keys := make(map[hotstuff.ID]consensus.PrivateKey)
	for _, nodeID := range scenario.Nodes {
		pk, ok := keys[nodeID.ReplicaID]
		if !ok {
			var err error
			pk, err = keygen.GenerateECDSAPrivateKey()
			if err != nil {
				return err
			}
			keys[nodeID.ReplicaID] = pk
		}
		node := Node{
			ID: nodeID,
		}
		builder := consensus.NewBuilder(nodeID.ReplicaID, pk)
		builder.Register(
			blockchain.New(),
			scenario.ConsensusCtor(),
			crypto.NewCache(ecdsa.New(), 100),
			synchronizer.New(testutil.FixedTimeout(scenario.ViewTimeout)),
			&configuration{
				node:    &node,
				network: network,
			},
			leaderRotation(scenario.Leaders),
			commandModule{commandGenerator: cg, node: &node},
		)
		node.Modules = builder.Build()
		network.Nodes[nodeID] = &node
		network.Replicas[nodeID.ReplicaID] = append(network.Replicas[nodeID.ReplicaID], &node)
	}
	return nil
}

func checkCommits(network *Network) (safe bool, commits int) {
	i := 0
	for {
		noCommits := true
		commitCount := make(map[consensus.Hash]int)
		for _, replica := range network.Replicas {
			if len(replica) != 1 {
				// TODO: should we be skipping replicas with twins?
				continue
			}
			if len(replica[0].ExecutedBlocks) <= i {
				continue
			}
			commitCount[replica[0].ExecutedBlocks[i].Hash()]++
			noCommits = false
		}
		// if all correct replicas have executed the same blocks, then there should be only one entry in commitCount
		// the number of replicas that committed the block could be smaller, if some correct replicas happened to
		// be in a different partition at the time when the test ended.
		if len(commitCount) != 1 {
			return false, i
		}

		if noCommits {
			break
		}

		i++
	}
	return true, i
}

type leaderRotation []hotstuff.ID

// GetLeader returns the id of the leader in the given view.
func (lr leaderRotation) GetLeader(view consensus.View) hotstuff.ID {
	// we start at view 1
	v := int(view) - 1
	if v >= 0 && v < len(lr) {
		return lr[v]
	}
	// default to 0 (which is an invalid id)
	return 0
}

type commandGenerator struct {
	mut     sync.Mutex
	nextCmd uint64
}

func (cg *commandGenerator) next() consensus.Command {
	cg.mut.Lock()
	defer cg.mut.Unlock()
	cmd := consensus.Command(strconv.FormatUint(cg.nextCmd, 10))
	cg.nextCmd++
	return cmd
}

type commandModule struct {
	commandGenerator *commandGenerator
	node             *Node
}

// Accept returns true if the replica should accept the command, false otherwise.
func (commandModule) Accept(_ consensus.Command) bool {
	return true
}

// Proposed tells the acceptor that the propose phase for the given command succeeded, and it should no longer be
// accepted in the future.
func (commandModule) Proposed(_ consensus.Command) {}

// Get returns the next command to be proposed.
// It may run until the context is cancelled.
// If no command is available, the 'ok' return value should be false.
func (cm commandModule) Get(_ context.Context) (cmd consensus.Command, ok bool) {
	return cm.commandGenerator.next(), true
}

// Exec executes the given command.
func (cm commandModule) Exec(block *consensus.Block) {
	cm.node.ExecutedBlocks = append(cm.node.ExecutedBlocks, block)
}

func (commandModule) Fork(block *consensus.Block) {}
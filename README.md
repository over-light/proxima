*Warning! This repository contains an ongoing development. Currently, it is in an alpha version. It definitely contains bugs.
The code should not be used in production!*

# Proxima: a DAG-based cooperative distributed ledger
Proxima is as decentralized and permissionless as Bitcoin (*proof-of-work*, PoW). 
<br>It is similar to *proof-of stake* (PoS), especially because it uses Sybil protection by token holdings.
<br>Yet it is neither PoW, nor a usual BFT-based PoS system. It is based on **cooperative consensus**. See 
- [Proxima whitepaper](https://arxiv.org/abs/2411.16456) 
- other [Proxima documents](https://lunfardo314.github.io/), which include:
   - [Overview of Proxima concepts](https://lunfardo314.github.io/#/overview/intro)
   - [Proxima transaction model](https://lunfardo314.github.io/#/txdocs/intro)
   - [Proxima ledger and programmability](https://lunfardo314.github.io/#/ledgerdocs/library)

## Testnet 

Please read instructions [how to join the open testnet](docs/testnet.md).

## Introduction
Proxima presents a novel architecture for a distributed ledger, commonly referred to as a "blockchain." 
The ledger of Proxima is organized in the form of directed acyclic graph (DAG) with UTXO transactions as vertices, 
rather than as a chain of blocks. It is a _transaction DAG_ (as opposed to the _blockDAG)_. Proxima does not use blocks and does not have a _mempool_. 
There's no need for block proposers. Due to the UTXO determinism, canonical ordering of transactions is natural, and there's no need for _sequencing_ of them in the sense of blockchains. 

Consensus on the state of ledger assets is achieved through the profit-driven behavior of token holders that are the only
category of participants in the network. This behavior is viable only when they cooperate by following the _biggest ledger coverage_ rule, 
similarly to the _longest chain rule_ in PoW blockchains. Consensus in Proxima is _probabilistic_, i.e., the finality is non-deterministic and subjective. 
Hence, **cooperative consensus**

The profit-and-consensus-seeking behavior is facilitated by enforcing purposefully designed UTXO transaction validity constraints.
The only prerequisite is liquidity of the token (i.e. market price of it). Thus, the participation of token holders in the network is completely permissionless by nature. 

Proxima distributed ledger does not require special categories of miners, validators, committees, or staking, with their functions, trust assumptions, and variety of interests.

In the proposed architecture, participants do not need knowledge about the global state of the system or the total order of ledger updates. 
The setup allows achieving high throughput and scalability alongside low transaction costs, while preserving key aspects of decentralization, 
open participation, and asynchronicity found in Bitcoin and other proof-of-work blockchains, but without unsustainable energy consumption. 

Sybil protection is achieved similarly to proof-of-stake blockchains, using tokens native to the ledger, yet the architecture operates in a multi-leader manner, 
without block proposers, committee selection, and staking.

Currently, the project is in an **ongoing development stage**. 

The repository contains a testnet version of the Proxima node. It is intended for experimental research and development. 
It also contains some tools, which includes basic wallet functionality.

## Highlights of the system's architecture
* **Fully permissionless**, undetermined, unbounded, generally unknown set of pseudonymous consensus participants. 
Participation in the ledger as a user is fully open, you only need to be present on the ledger as a token holder. There is no need for any kind of permissions, registration, committee selection or voting processes.
* **Token holders are the only category of participants**. No miners, no validators, no committees nor any other paid third parties with their interest.
* Sybil protection is token-based (like PoS). Influence of the participant is proportional to the amount of its holdings, i.e., to its _skin-in-the-game_.
* **Liquidity of the token** (liquid market price) is the only condition for the system to be fully permissionless. Ownership of tokens is the only prerequisite for the participation in any role. 
* *No ASICs, no GPUs, no mining pools**
* **Multi-leader**. The system operates without selecting a consensus leader or block proposers, providing a more decentralized approach. It can also be called _leaderless_.
* **Nash equilibrium**. It is achieved with the optimal strategy of **biggest ledger coverage rule**, which is analogous to the Bitcoin's _longest chain rule_ in PoW blockchains.
* The behavior of consensus participants does not use any "consensus rounds", does not rely on the knowledge about the existing peers, their state or voting/opinions, is "oblivious" to the communication history.
* Unlike in blockchains, the optimal strategy is **cooperative**, rather than **competitive**. Consensus is achieved by cooperation between actors rather than
choosing one wining proposal (leader) among many. This facilitates social consensus among participants
* Goal of the consensus id conflict resolution. There's no need for sequencing. Canonic order among UTXO transactions naturally comes after conflict (double-spend) resolution. 
This is the opposite to blockchains, which require total oder (_sequencing_) for determinism and to prevent double-spending. 
* **High throughput**, as a result of **massive parallelism** and **absence of global bottlenecks**
* **High level of decentralization**, among the highest achievable in distributed ledgers (together with PoW principle)
* **Low energy requirements**, unlike PoW. 
* **Low cost per transaction**, like PoS
* The only type of message between participants is raw transaction.
* **Asynchrony**. Big delays may result in network partitioning and forks, which will be perceived by users as inoperational network. 
After communications restored, forks will be merged by the *biggest ledger coverage* rule and network will auto-restore it operation: will "self-heal". 
* Peers seek cooperation. They are incentivized to be as close to other big token hodlers, as possible. 
Otherwise, their transactions may get orphaned, or they will lose opportunity to win periodical inflation bonus lottery. 
* Participants are incentivized to maintain their clock approximately synchronized with the global clock. 
* **Probabilistic finality**. Depends on subjective assumptions by the users, similar to 6-block rule in Bitcoin. Normally 1 to 3 slots (10 to 30 sec) is enough. Theoretical bounds depend on network latency. 
Due to the UTXO transaction determinism, there's no need to wait for the confirmation of the previous transaction. Therefore, transactions can be issued in batches or streams.
* **Consensus rule is local and "oblivious"**. It does not require any global knowledge of the dynamic system state, such as composition of the committee, assumptions about node weights/stakes, or other registries.
* **1-tier trust assumptions**. Only token holders are involved in the process. It is different from the multiple layers of trust required in other 
blockchain systems such as PoW (which includes users and miners) and PoS (which includes at least users, block proposers, committee/leader selection procedure, and the committee). 
* One category of participants also removes misalignment of interests between different categories of participants (e.g. "hodlers vs miners").
* **Parallelism at the distributed consensus level**. Assets converge to their finality states in parallel, yet cooperating with each other.
* **Parallelism** at the node level. All transactions are processed in parallel on each node.
* **Spamming prevention** is based on the transaction rate limits per user (token holder): at the ledger level and at the pre-ledger buffer (_memDAG_) level
* **Simpler than most**, except Bitcoin. The above facilitates clear and relatively simple overall concept and node architecture, 
much simpler than most PoS and DAG-based systems, which are usually complex in their consensus layer. 

## Further information
* [Technical whitepaper (pdf)](https://arxiv.org/abs/2411.16456) contains a detailed description of the *cooperative ledger* concept
* [Simplified presentation of Proxima concepts](https://hackmd.io/@Evaldas/Sy4Gka1DC) skips many technical details and adds more pictures
* Tutorials and instructions (outdated somehow):
  * [CLI wallet program `proxi`](docs/proxi.md)
  * [Running first node in the network](docs/run_boot.md)
  * [Running access node](docs/run_access.md)
  * [Running node with sequencer](docs/run_sequencer.md)
  * [Running small testnet in Docker](tests/docker/docker-network.md)
  * [Delegation in `proxi`](docs/delegate.md)
  * [How to join testnet?](docs/testnet.md)
* Introductory videos:
  * [1. Introduction. Principles of Nakamoto consensus](https://youtu.be/qDnjnrOJK_g)
  * [2. UTXO tangle. Ledger coverage](https://youtu.be/CT0_FlW-ObM)
  * [3. Cooperative consensus](https://youtu.be/7N_L6CMyRdo)

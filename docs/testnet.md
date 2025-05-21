## Participating in the open testnet

Proxima testnet is an experimental network, intended for testing node software and various aspects of the Proxima concept.

We have been running several of them, each with at least 9 nodes and 5 sequencers among them. Normally we aim to control testnets
by owning a majority of token supply. This is due to the experimental nature of the networks and frequent breaking changes. 
After each breaking change, we have to reset the ledger state from genesis.

Starting from version `v0.1.2` the testnet is open. It means everybody can join the network with the access node,
everybody with enough tokens can run a sequencer and earn inflation.

Starting from version `v0.1.3-testnet` network has faucet. You can receive tokens to your address by calling node API. 

Starting from version `v0.1.4-testnet`, the node has _delegation_ function implemented. It allows any token holder to participate
in the consensus and earn inflation by delegating their holdings to a sequencer **without the need to run a sequencer**. 
(note that minimum token amount limits are applied).

### Other docs
Please read at least basic docs on [proxi](proxi.md), [delegation](delegate.md) and other available materials. 
At this stage Proxima lacks proper documentation.

### Public access points
These are public API endpoints to access from `proxi` or for other purposes:

* http://113.30.191.219:8001
* http://63.250.56.190:8001
* http://83.229.84.197:8001
* http://5.180.181.103:8001

The faucet is available on `113.30.191.219:9500`. You need the following section in your `proxi.yaml`:

```yaml
faucet:
    port: 9500
    addr: 113.30.191.219
```

### How to get tokens?
Use command `proxi node getfunds` to get tokens to your wallet as defined by the `proxi.yaml` in you current directory.
You can do it once per day. Faucet will send generous `1.000.000.000.000` tokens to your account. 

Check you balance with command `proxi node balance`. If everything is ok, the requested tokens will come after 20-30 seconds. 

This amount is ~0.1% of the total supply. It is enough to run sequencer and for delegation.

### What can you do with your tokens?

#### Transfer tokens between accounts

To send tokens between accounts, you use command `proxi node transfer`. See [proxi docs](proxi.md). 
Note, that for this `proxi.yaml` must be configured properly. In particular, _tag-along sequencer_ and _tag-along fees_ must be
configured properly. You can list all sequencers with command `proxi node allchains -q` and choose one of them as tag-along. 

#### Earn inflation by delegation
Please read [delegation](delegate.md). It is **strongly encouraged** to delegate all but some minimum amount (say `1.000.000`) of your tokens, 
immediately you receive them with `proxi node getfunds`. 

All sequencers with delegation information can be listed with `proxi node allchains -d -q`. It is easy to choose one of them for delegation
and tag-along.

Your delegated tokens will contribute to the security of the network and, in exchange, will earn you inflation around **10% annually**. 
If your tokens remain passive in your normal account (which has address in the form `a(0x<hex>)`), you will not receive any inflation. 

#### Earn inflation by running sequencer
To run a sequencer, you need two things:
* to run an _access node_. See [Running access node](run_access.md) for detailed instructions. Note that it is pretty easy to run access node
and this does not require owning any tokens, but does not contribute to the security of the network.
* configure and run a **sequencer** on that access node (then we call it _sequencer node_). See [Running node with the sequencer](run_sequencer.md). To run a sequencer you will need tokens.  
Sequencers are programs that generate inflation and therefore contribute to the security of the network on behalf of the token holder. 
Sequencers more than delegated tokens because they participate in the lottery for the *branch inflation bonus*. In addition to that, 
sequencers collect tag-along fees and delegation margin.

### Disclaimer

We will do our best to help you on the Proxima Discord channel `#testnet`. Please note, however, our resources are very limited. 
We count on growing community which can help each other.

Please also note that:

* tokens are fake. They have 0 value and are only for testing.
* the Proxima software at this stage is experimental and definitely contains bugs. Do not use it in production! 


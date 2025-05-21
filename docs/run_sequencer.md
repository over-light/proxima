## Running node with the sequencer

**Sequencer** is a program run as a part of a node. It is building **sequencer chain** by consolidating chains of
other sequencers and by consuming tag-along outputs sent to it. 

Other users send tag-along fees to sequencers in their transactions so that those transactions to be pulled into
the state after the sequencer consumes tag-along output.

The core of the node supports multi-sequencer mode; however, in the current version we are limiting the 
number of sequencers in the node to one.

- if a node runs a sequencer, it is a _sequencer node_ 
- if a node does not run a sequencer, it is an _access node_ 

To run a sequencer in the testnet, one needs at least 1.000.000.000 tokens (1 millionth of the initial supply). 
This amount is intentionally made smaller for the testnet. 
A reasonable amount for the real network would be 1/1000 of the initial supply. It would limit number of sequencers on the network. 

### Steps to run the sequencer:

1. make your access node running and synced. See instructions in [Running access node](run_access.md)
1. create a new chain origin with `proxi node mkchain <amount>`. Make sure you don't use the whole amount balance for the chain.
It is recommended to leave at least a few millions of tokens for tag-along fees, spamming and other purposes.
1. once you created chain origin, you can check it with `proxi node mychains`
1. configure `sequencer` section in the node configuration profile `proxima.yaml` of your access node the following way:

```yaml
sequencer:
  enable: true
  name: <sequencer name>
  chain_id: <chain ID>
  controller_key: <private key hex>
  pace: 5
```

With `enable: true/false` you can enable or disable the start of the sequencer at the startup of the node. With `enable: false`
node is just an access node.

_sequencer name_ is any mnemonic name used for the sequencer. It will appear in the logs and in the sequencer transactions.
It is recommended to have it no longer than 4-6 characters. 

_chain ID_ is the ID of the newly created chain (hex encoded, not `$/` prefix). It is also called _sequencer ID_.

_private key hex_ is the controlling private key. Sequencer will use it to sign transactions. Copy it from your wallet config
profile `proxi.yaml`

`pace` parameter is minimum number of ticks between two subsequent sequencers transactions. In the testnet version 
it should not be less than `3` and not exceed `20` or so. 1 tick is 80 milliseconds on the clock-time scale.

- start the node as usually. Node will log details of the sequencer. It will take 10 to 15 seconds until sequencer starts
issuing sequencer transactions and earning inflation with branch inflation bonus (when lucky).

- adjust your wallet profile `proxi.yaml` by putting your _sequencer ID_ as own (controlled) sequencer in `wallet.sequencer_id`. 
With this configured properly, you will be able to withdraw part of your funds from the running sequencer chain 
without stopping the sequencer with command `proxi node seq withdraw <amount> [-t <options targe address>]`.
Note that every transaction costs fees. So, it is smart to configure your wallet's tag-along sequencer to your own sequencer.
This way all the fees will go to yourself: making your transactions essentially fee-less. 

### Useful 
Configuration key `logger.verbosity` specifies logging level for the sequencer transaction:

`logger.verbosity: 0` only branch transactions are displayed in the log

`logger.verbosity: 1` branch and other sequencer transactions are displayed in the log


## Future optimizations at ledger and state level (breaking)

* In the *transaction ID*: Change ticks from 256 to 128 per slot, move sequencer flag to the end of timestamp prefix, double the maximum number of slots.
That would simplify sorting and optimize trie size
* change the amount format to varint or similar
* maintain UTXO bit map together with transaction ID record in the state. That would optimize checking if input is in the state
* make chain ID 24 bytes (?)
* ledger library:
  * detach ledger library definitions form the node's code. Make definitions more controllable
  * move ledger ID constants to the library definition. 
  * move easyfl library to the state.
  * make easyfl library upgradeable
* Move ledger and multi-state DB to separate repository
* Introduce more open transaction signature part to different signature schemes, like BLS and similar. 
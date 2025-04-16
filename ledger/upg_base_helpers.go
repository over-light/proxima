package ledger

const _upgradeBaseHelpers = `
# definitions of base helper functions
functions:
   -
      sym: mustSize
      source: if(equalUint(len($0), $1), $0, !!!wrong_data_size)
   -
      sym: mustValidTimeTick
      source: if(and(equalUint(len($0),1), lessThan(uint8Bytes($0),constMaxTickValuePerSlot) ), $0, !!!wrong_ticks_value)
   -
      sym: mustValidTimeSlot
      description: returns $0 result if $0 can be interpreted as slot value, otherwise returns 0x 
      source: if(equalUint(len($0), timeSlotSizeBytes), $0, !!!wrong_slot_data)
   -
      sym: mul8
      description: last byte of big-endian uint64 multiplication result
      source: byte(mul($0,$1),7)
   -
      sym: div8
      description: last byte of big-endian uint64 integer division result
      source: byte(div($0,$1),7)
   -
      sym: timestampBytes
      description: validates and composes $0 as slot value and $1 as ticks value into timestamp
      source: concat(mustValidTimeSlot($0),mul8(mustValidTimeTick($1),2))
   -
      sym: first4Bytes
      description: first 4 bytes of $0
      source: slice($0, 0, 3)
   -
      sym: first5Bytes
      description: first 5 bytes of $0
      source: slice($0, 0, 4)
   -
      sym: timestampBytesFromPrefix
      description: nullifies sequencer bit in the prefix and thus makes a timestamp from a txid  
      source: bitwiseAND(first5Bytes($0), 0xfffffffff6)
   -
      sym: timeTickFromTimestampBytes
      description: returns ticks of the timestamp
      source: div8(byte($0, 4),2)
   -
      sym: isTimestampBytesOnSlotBoundary
      description: returns non-empty value if ticks of the $0 timestamp are 0
      source: isZero(timeTickFromTimestampBytes($0))
`

package p200

import (
	"github.com/golang/geo/r3"
	"github.com/irmine/gomine/net/info"
	"github.com/irmine/gomine/net/packets"
	"github.com/irmine/worlds/entities/data"
)

type AddEntityPacket struct {
	*packets.Packet
	UniqueId   int64
	RuntimeId  uint64
	EntityType uint32
	Position   r3.Vector
	Motion     r3.Vector
	Rotation   data.Rotation

	Attributes data.AttributeMap
	EntityData map[uint32][]interface{}
}

func NewAddEntityPacket() *AddEntityPacket {
	return &AddEntityPacket{packets.NewPacket(info.PacketIds200[info.AddEntityPacket]), 0, 0, 0, r3.Vector{}, r3.Vector{}, data.Rotation{}, data.NewAttributeMap(), nil}
}

func (pk *AddEntityPacket) Encode() {
	pk.PutUniqueId(pk.UniqueId)
	pk.PutRuntimeId(pk.RuntimeId)
	pk.PutUnsignedVarInt(pk.EntityType)
	pk.PutVector(pk.Position)
	pk.PutVector(pk.Motion)
	pk.PutRotation(pk.Rotation, false)
	pk.PutAttributeMap(pk.Attributes)
	pk.PutEntityData(pk.EntityData)
	pk.PutUnsignedVarInt(0)
}

func (pk *AddEntityPacket) Decode() {
	pk.UniqueId = pk.GetUniqueId()
	pk.RuntimeId = pk.GetRuntimeId()
	pk.EntityType = pk.GetUnsignedVarInt()
	pk.Position = pk.GetVector()
	pk.Motion = pk.GetVector()
	pk.Rotation = pk.GetRotation(false)
	pk.Attributes = pk.GetAttributeMap()
	pk.EntityData = pk.GetEntityData()
}
package worlds

import (
	"github.com/bedrock-tool/bedrocktool/utils/behaviourpack"
	"github.com/bedrock-tool/bedrocktool/utils/nbtconv"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type entityState struct {
	RuntimeID  uint64
	UniqueID   int64
	EntityType string

	Position         mgl32.Vec3
	Pitch, Yaw       float32
	HeadYaw, BodyYaw float32
	Velocity         mgl32.Vec3

	Metadata  protocol.EntityMetadata
	Inventory map[byte]map[byte]protocol.ItemInstance

	Helmet     *protocol.ItemInstance
	Chestplate *protocol.ItemInstance
	Leggings   *protocol.ItemInstance
	Boots      *protocol.ItemInstance
}

type serverEntityType struct {
	Encoded string
	NBT     map[string]any
}

func (t serverEntityType) EncodeEntity() string {
	return t.Encoded
}

func (t serverEntityType) BBox(e world.Entity) cube.BBox {
	return cube.Box(-0.5, 0, -0.5, 0.5, 1, 0.5)
}

func (t serverEntityType) DecodeNBT(m map[string]any) world.Entity {
	return nil // not implemented, and never should
}

func (t serverEntityType) EncodeNBT(e world.Entity) map[string]any {
	return t.NBT
}

type serverEntity struct {
	world.Entity
	EntityType serverEntityType
}

var _ world.SaveableEntityType = &serverEntityType{}

func (e serverEntity) Type() world.EntityType {
	return e.EntityType
}

func (w *worldsHandler) processAddActor(pk *packet.AddActor) {
	e, ok := w.worldState.entities[pk.EntityRuntimeID]
	if !ok {
		e = &entityState{
			RuntimeID:  pk.EntityRuntimeID,
			UniqueID:   pk.EntityUniqueID,
			EntityType: pk.EntityType,
			Inventory:  make(map[byte]map[byte]protocol.ItemInstance),
			Metadata:   make(map[uint32]any),
		}
		w.worldState.entities[pk.EntityRuntimeID] = e

		w.bp.AddEntity(behaviourpack.EntityIn{
			Identifier: pk.EntityType,
			Attr:       pk.Attributes,
			Meta:       pk.EntityMetadata,
		})
	}

	e.Position = pk.Position
	e.Pitch = pk.Pitch
	e.Yaw = pk.Yaw
	e.BodyYaw = pk.BodyYaw
	e.HeadYaw = pk.HeadYaw
	e.Velocity = pk.Velocity

	for k, v := range pk.EntityMetadata {
		e.Metadata[k] = v
	}
}

var flagNames = map[uint8]string{
	protocol.EntityDataFlagSheared:      "Sheared",
	protocol.EntityDataFlagCaptain:      "IsIllagerCaptain",
	protocol.EntityDataFlagSitting:      "Sitting",
	protocol.EntityDataFlagBaby:         "IsBaby",
	protocol.EntityDataFlagTamed:        "IsTamed",
	protocol.EntityDataFlagTrusting:     "IsTrusting",
	protocol.EntityDataFlagOrphaned:     "IsOrphaned",
	protocol.EntityDataFlagAngry:        "IsAngry",
	protocol.EntityDataFlagOutOfControl: "IsOutOfControl",
	protocol.EntityDataFlagSaddled:      "Saddled",
	protocol.EntityDataFlagChested:      "Chested",
	protocol.EntityDataFlagShowBottom:   "ShowBottom",
	protocol.EntityDataFlagGliding:      "IsGliding",
	protocol.EntityDataFlagSwimming:     "IsSwimming",
	protocol.EntityDataFlagEating:       "IsEating",
	protocol.EntityDataFlagScared:       "IsScared",
	protocol.EntityDataFlagStunned:      "IsStunned",
	protocol.EntityDataFlagRoaring:      "IsRoaring",
}

func entityMetadataToNBT(metadata protocol.EntityMetadata, nbt map[string]any) {
	if variant, ok := metadata[protocol.EntityDataKeyVariant]; ok {
		nbt["Variant"] = variant
	}
	if markVariant, ok := metadata[protocol.EntityDataKeyMarkVariant]; ok {
		nbt["MarkVariant"] = markVariant
	}
	if color, ok := metadata[protocol.EntityDataKeyColorIndex]; ok {
		nbt["Color"] = color
	}
	if color2, ok := metadata[protocol.EntityDataKeyColorTwoIndex]; ok {
		nbt["Color2"] = color2
	}
	if skinID, ok := metadata[protocol.EntityDataKeySkinID]; ok {
		nbt["SkinID"] = int32(skinID.(int32))
	}

	if name, ok := metadata[protocol.EntityDataKeyName]; ok {
		nbt["CustomName"] = name
	}
	if ShowNameTag, ok := metadata[protocol.EntityDataKeyAlwaysShowNameTag]; ok {
		if ShowNameTag != 0 {
			nbt["CustomNameVisible"] = true
		} else {
			nbt["CustomNameVisible"] = false
		}
	}

	if _, ok := metadata[protocol.EntityDataKeyFlags]; ok {
		if metadata.Flag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagNoAI) {
			nbt["IsAutonomous"] = false
		}
		for k, v := range flagNames {
			nbt[v] = metadata.Flag(protocol.EntityDataKeyFlags, k)
		}

		AlwaysShowName := metadata.Flag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagAlwaysShowName)
		if AlwaysShowName {
			nbt["CustomNameVisible"] = true
		}

		type effect struct {
			Id                              byte
			Amplifier                       byte
			Duration                        int32
			DurationEasy                    int32
			DurationNormal                  int32
			DurationHard                    int32
			Ambient                         bool
			ShowParticles                   bool
			DisplayOnScreenTextureAnimation bool
		}

		activeEffects := []effect{}
		addEffect := func(id int, showParticles bool) {
			activeEffects = append(activeEffects, effect{
				Id:            byte(id),
				Amplifier:     1,
				Duration:      10000000,
				ShowParticles: false,
			})
		}

		invisible := metadata.Flag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagInvisible)
		if invisible {
			addEffect(packet.EffectInvisibility, false)
		}

		if len(activeEffects) > 0 {
			nbt["ActiveEffects"] = activeEffects
		}
	}
}

func vec3float32(x mgl32.Vec3) []float32 {
	return []float32{float32(x[0]), float32(x[1]), float32(x[2])}
}

func (s *entityState) ToServerEntity() serverEntity {
	e := serverEntity{
		EntityType: serverEntityType{
			Encoded: s.EntityType,
			NBT: map[string]any{
				"Pos":      vec3float32(s.Position),
				"Rotation": []float32{s.HeadYaw, s.Pitch},
				"Motion":   vec3float32(s.Velocity),
				"UniqueID": int64(s.UniqueID),
			},
		},
	}
	entityMetadataToNBT(s.Metadata, e.EntityType.NBT)

	if s.Helmet != nil || s.Chestplate != nil || s.Leggings != nil || s.Boots != nil {
		e.EntityType.NBT["Armor"] = []map[string]any{
			nbtconv.WriteItem(stackToItem(s.Helmet.Stack), true),
			nbtconv.WriteItem(stackToItem(s.Chestplate.Stack), true),
			nbtconv.WriteItem(stackToItem(s.Leggings.Stack), true),
			nbtconv.WriteItem(stackToItem(s.Boots.Stack), true),
		}
	}
	return e
}

func (w *worldsHandler) ProcessEntityPackets(pk packet.Packet) packet.Packet {
	if !w.settings.SaveEntities {
		return pk
	}

	switch pk := pk.(type) {
	case *packet.AddActor:
		w.processAddActor(pk)
	case *packet.RemoveActor:
	case *packet.SetActorData:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			e.Metadata = pk.EntityMetadata
			w.bp.AddEntity(behaviourpack.EntityIn{
				Identifier: e.EntityType,
				Attr:       nil,
				Meta:       pk.EntityMetadata,
			})
		}
	case *packet.SetActorMotion:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			e.Velocity = pk.Velocity
		}
	case *packet.MoveActorDelta:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			if pk.Flags&packet.MoveActorDeltaFlagHasX != 0 {
				e.Position[0] = pk.Position[0]
			}
			if pk.Flags&packet.MoveActorDeltaFlagHasY != 0 {
				e.Position[1] = pk.Position[1]
			}
			if pk.Flags&packet.MoveActorDeltaFlagHasZ != 0 {
				e.Position[2] = pk.Position[2]
			}
			if pk.Flags&packet.MoveActorDeltaFlagHasRotX != 0 {
				e.Pitch = pk.Rotation.X()
			}
			if pk.Flags&packet.MoveActorDeltaFlagHasRotY != 0 {
				e.Yaw = pk.Rotation.Y()
			}
			//if pk.Flags&packet.MoveActorDeltaFlagHasRotZ != 0 {
			// no roll
			//}
		}
	case *packet.MoveActorAbsolute:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			e.Position = pk.Position
			e.Pitch = pk.Rotation.X()
			e.Yaw = pk.Rotation.Y()
		}
	case *packet.MobEquipment:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			w, ok := e.Inventory[pk.WindowID]
			if !ok {
				w = make(map[byte]protocol.ItemInstance)
				e.Inventory[pk.WindowID] = w
			}
			w[pk.HotBarSlot] = pk.NewItem
		}
	case *packet.MobArmourEquipment:
		e, ok := w.worldState.entities[pk.EntityRuntimeID]
		if ok {
			e.Helmet = &pk.Helmet
			e.Chestplate = &pk.Chestplate
			e.Leggings = &pk.Chestplate
			e.Boots = &pk.Boots
		}
	}
	return pk
}
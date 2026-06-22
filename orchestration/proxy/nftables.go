package proxy

import (
	"fmt"
	"net/netip"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// poolMapName is the nft map the kernel DNATs through: mark -> backend ip. The
// handler stamps a backend's mark on a packet (see handler.go); the execute chain
// looks the mark up here and rewrites the destination to that ip. The backend
// port is the constant backendPort for CS2 (27015).
const poolMapName = "pool"

// installTable (re)creates `table ip <name>` with the two-chain data path that
// splits the decision (Go) from the execution (kernel):
//
//	table ip <name> {
//	    map pool { type mark : ipv4_addr ; }
//
//	    # DECIDE: queue the first packet of every new UDP flow to Go, which runs
//	    # round-robin/session/generation logic and stamps a backend mark on it.
//	    chain decide {
//	        type filter hook prerouting priority mangle;   # -150
//	        ct state new meta l4proto udp queue num <n> bypass
//	    }
//
//	    # EXECUTE: the kernel does the REAL dnat from the mark Go set. Because it
//	    # runs at a lower-numbered priority it is the next base chain the packet
//	    # hits after the queue verdict re-injects it, mark still attached. A real
//	    # nf_nat binding means conntrack reverse-NATs the replies on its own. The
//	    # 'meta l4proto udp' match is mandatory: a DNAT that rewrites a port is a
//	    # "transport protocol mapping", which nft only allows after an l4 match.
//	    chain execute {
//	        type nat hook prerouting priority dstnat;       # -100
//	        meta l4proto udp meta mark != 0 dnat to meta mark map @pool : 27015
//	    }
//	}
//
// The table is static after this: backends are added/removed as @pool elements
// (addPoolElement / delPoolElement), never by touching chains or rules. installTable
// stores the table and map handles on the Proxy for those later element ops.
func (p *Proxy) installTable() error {
	deleteTable(p.cfg.Table)

	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables conn: %w", err)
	}

	t := c.AddTable(&nftables.Table{Name: p.cfg.Table, Family: nftables.TableFamilyIPv4})

	// map pool { type mark : ipv4_addr ; }
	pool := &nftables.Set{
		Name:     poolMapName,
		Table:    t,
		IsMap:    true,
		KeyType:  nftables.TypeMark,
		DataType: nftables.TypeIPAddr,
	}
	if err := c.AddSet(pool, nil); err != nil {
		return fmt.Errorf("add map %s: %w", poolMapName, err)
	}

	// chain decide: ct state new && udp -> queue num <n> bypass
	decide := c.AddChain(&nftables.Chain{
		Name:     "decide",
		Table:    t,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityMangle,
	})
	c.AddRule(&nftables.Rule{
		Table: t,
		Chain: decide,
		Exprs: []expr.Any{
			// ct state new
			&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitNEW),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0, 0, 0, 0}},
			// meta l4proto udp
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_UDP}},
			// queue num <n> bypass
			&expr.Queue{Num: p.cfg.QueueNum, Flag: expr.QueueFlagBypass},
		},
	})

	// chain execute: meta l4proto udp meta mark != 0 dnat to meta mark map @pool : <backendPort>
	execCh := c.AddChain(&nftables.Chain{
		Name:     "execute",
		Table:    t,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityNATDest,
	})
	c.AddRule(&nftables.Rule{
		Table: t,
		Chain: execCh,
		Exprs: []expr.Any{
			// meta l4proto udp
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_UDP}},
			// meta mark (kept in reg 1 for both the != 0 test and the lookup key)
			&expr.Meta{Key: expr.MetaKeyMARK, Register: 1},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0, 0, 0, 0}},
			// reg1 (mark) -> @pool -> reg1 = backend ip
			&expr.Lookup{
				SourceRegister: 1,
				DestRegister:   1,
				IsDestRegSet:   true,
				SetName:        pool.Name,
				SetID:          pool.ID,
			},
			// reg2 = the constant backend port
			&expr.Immediate{Register: 2, Data: binaryutil.BigEndian.PutUint16(backendPort)},
			// dnat to ip (reg1) : port (reg2)
			&expr.NAT{
				Type:        expr.NATTypeDestNAT,
				Family:      unix.NFPROTO_IPV4,
				RegAddrMin:  1,
				RegProtoMin: 2,
			},
		},
	})

	if err := c.Flush(); err != nil {
		return fmt.Errorf("install table ip %s: %w", p.cfg.Table, err)
	}

	p.nftTable = t
	p.nftPool = pool
	return nil
}

// deleteTable removes 'table ip <name>' and everything in it (chains, rules, the
// pool map) in one operation. A missing table is not an error, so this is safe to
// call on a clean host.
func deleteTable(name string) {
	c, err := nftables.New()
	if err != nil {
		return
	}
	c.DelTable(&nftables.Table{Name: name, Family: nftables.TableFamilyIPv4})
	c.Flush()
}

// addPoolElement adds (or replaces) the @pool entry mark -> backend ip, so the
// execute chain DNATs marked packets to this backend. mark is the backend's
// generation; the key is native-endian (meta mark loads skb->mark host-order) and
// the value is the 4-byte network-order address.
func (p *Proxy) addPoolElement(mark uint32, ip netip.Addr) error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables conn: %w", err)
	}
	a := ip.As4()
	if err := c.SetAddElements(p.nftPool, []nftables.SetElement{{
		Key: binaryutil.NativeEndian.PutUint32(mark),
		Val: a[:],
	}}); err != nil {
		return fmt.Errorf("add pool element mark %d: %w", mark, err)
	}
	if err := c.Flush(); err != nil {
		return fmt.Errorf("add pool element mark %d: %w", mark, err)
	}
	return nil
}

// delPoolElement removes the @pool entry for mark. The kernel stops DNATing
// packets carrying it, but Go never stamps a removed backend's mark anyway, so
// this just keeps the map tidy.
func (p *Proxy) delPoolElement(mark uint32) error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nftables conn: %w", err)
	}
	if err := c.SetDeleteElements(p.nftPool, []nftables.SetElement{{
		Key: binaryutil.NativeEndian.PutUint32(mark),
	}}); err != nil {
		return fmt.Errorf("delete pool element mark %d: %w", mark, err)
	}
	if err := c.Flush(); err != nil {
		return fmt.Errorf("delete pool element mark %d: %w", mark, err)
	}
	return nil
}

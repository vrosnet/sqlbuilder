// Copyright 2015 CoreOS, Inc. All rights reserved.
// Copyright 2014 Dropbox, Inc. All rights reserved.
// Use of this source code is governed by the BSD 3-Clause license,
// which can be found in the LICENSE file.

package sqlbuilder

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
)

// XXX: Maybe add UIntColumn

// Representation of a table for query generation
type Column interface {
	isProjectionInterface

	Name() string
	// Serialization for use in column lists
	SerializeSqlForColumnList(includeTableName bool, d Dialect, out *bytes.Buffer) error
	// Serialization for use in an expression (Clause)
	SerializeSql(d Dialect, out *bytes.Buffer) error

	// Internal function for tracking table that a column belongs to
	// for the purpose of serialization
	setTableName(table string) error
}

type NullableColumn bool

const (
	Nullable    NullableColumn = true
	NotNullable NullableColumn = false
)

// A column that can be refer to outside of the projection list
type NonAliasColumn interface {
	Column
	isOrderByClauseInterface
	isExpressionInterface
}

type Collation string

const (
	UTF8CaseInsensitive Collation = "utf8_unicode_ci"
	UTF8CaseSensitive   Collation = "utf8_unicode"
	UTF8Binary          Collation = "utf8_bin"
)

// Representation of MySQL charsets
type Charset string

const (
	UTF8 Charset = "utf8"
)

// The base type for real materialized columns.
type baseColumn struct {
	isProjection
	isExpression
	name     string
	nullable NullableColumn
	table    string
}

func (c *baseColumn) Name() string {
	return c.name
}

func (c *baseColumn) setTableName(table string) error {
	c.table = table
	return nil
}

func (c *baseColumn) SerializeSqlForColumnList(includeTableName bool, d Dialect, out *bytes.Buffer) error {
	if c.table != "" && includeTableName {
		out.WriteRune(d.EscapeCharacter())
		out.WriteString(c.table)
		out.WriteRune(d.EscapeCharacter())
		out.WriteByte('.')
	}
	out.WriteRune(d.EscapeCharacter())
	out.WriteString(c.name)
	out.WriteRune(d.EscapeCharacter())
	return nil
}

func (c *baseColumn) SerializeSql(d Dialect, out *bytes.Buffer) error {
	return c.SerializeSqlForColumnList(true, d, out)
}

type bytesColumn struct {
	baseColumn
	isExpression
}

// Representation of VARBINARY/BLOB columns
// This function will panic if name is not valid
func BytesColumn(name string, nullable NullableColumn) NonAliasColumn {
	if !validIdentifierName(name) {
		panic("Invalid column name in bytes column")
	}
	bc := &bytesColumn{}
	bc.name = name
	bc.nullable = nullable
	return bc
}

type stringColumn struct {
	baseColumn
	isExpression
	charset   Charset
	collation Collation
}

// Representation of VARCHAR/TEXT columns
// This function will panic if name is not valid
func StrColumn(
	name string,
	charset Charset,
	collation Collation,
	nullable NullableColumn) NonAliasColumn {

	if !validIdentifierName(name) {
		panic("Invalid column name in str column")
	}
	sc := &stringColumn{charset: charset, collation: collation}
	sc.name = name
	sc.nullable = nullable
	return sc
}

type dateTimeColumn struct {
	baseColumn
	isExpression
}

// Representation of DateTime columns
// This function will panic if name is not valid
func DateTimeColumn(name string, nullable NullableColumn) NonAliasColumn {
	if !validIdentifierName(name) {
		panic("Invalid column name in datetime column")
	}
	dc := &dateTimeColumn{}
	dc.name = name
	dc.nullable = nullable
	return dc
}

type integerColumn struct {
	baseColumn
	isExpression
}

// Representation of any integer column
// This function will panic if name is not valid
func IntColumn(name string, nullable NullableColumn) NonAliasColumn {
	if !validIdentifierName(name) {
		panic("Invalid column name in int column")
	}
	ic := &integerColumn{}
	ic.name = name
	ic.nullable = nullable
	return ic
}

type doubleColumn struct {
	baseColumn
	isExpression
}

// Representation of any double column
// This function will panic if name is not valid
func DoubleColumn(name string, nullable NullableColumn) NonAliasColumn {
	if !validIdentifierName(name) {
		panic("Invalid column name in int column")
	}
	ic := &doubleColumn{}
	ic.name = name
	ic.nullable = nullable
	return ic
}

type booleanColumn struct {
	baseColumn
	isExpression

	// XXX: Maybe allow isBoolExpression (for now, not included because
	// the deferred lookup equivalent can never be isBoolExpression)
}

// Representation of TINYINT used as a bool
// This function will panic if name is not valid
func BoolColumn(name string, nullable NullableColumn) NonAliasColumn {
	if !validIdentifierName(name) {
		panic("Invalid column name in bool column")
	}
	bc := &booleanColumn{}
	bc.name = name
	bc.nullable = nullable
	return bc
}

type aliasColumn struct {
	baseColumn
	expression Expression
}

func (c *aliasColumn) SerializeSql(d Dialect, out *bytes.Buffer) error {
	out.WriteRune(d.EscapeCharacter())
	out.WriteString(c.name)
	out.WriteRune(d.EscapeCharacter())
	return nil
}

func (c *aliasColumn) SerializeSqlForColumnList(includeTableName bool, d Dialect, out *bytes.Buffer) error {
	if !validIdentifierName(c.name) {
		return fmt.Errorf(
			"invalid alias name `%s`. Generated sql: %s",
			c.name,
			out.String(),
		)
	}
	if c.expression == nil {
		return errors.New("cannot alias a nil expression. Generated sql: " + out.String())
	}

	out.WriteByte('(')
	if c.expression == nil {
		return errors.New("nil alias clause. Generate sql: " + out.String())
	}
	if err := c.expression.SerializeSql(d, out); err != nil {
		return err
	}
	out.WriteString(") AS ")
	out.WriteRune(d.EscapeCharacter())
	out.WriteString(c.name)
	out.WriteRune(d.EscapeCharacter())
	return nil
}

func (c *aliasColumn) setTableName(table string) error {
	return fmt.Errorf("alias column '%s' should never have setTableName called on it", c.name)
}

// Representation of aliased clauses (expression AS name)
func Alias(name string, c Expression) Column {
	ac := &aliasColumn{}
	ac.name = name
	ac.expression = c
	return ac
}

// This is a strict subset of the actual allowed identifiers
var validIdentifierRegexp = regexp.MustCompile("^[a-zA-Z_]\\w*$")

// Returns true if the given string is suitable as an identifier.
func validIdentifierName(name string) bool {
	return validIdentifierRegexp.MatchString(name)
}

// Pseudo Column type returned by table.C(name)
type deferredLookupColumn struct {
	isProjection
	isExpression
	table   *Table
	colName string

	cachedColumn NonAliasColumn
}

func (c *deferredLookupColumn) Name() string {
	return c.colName
}

func (c *deferredLookupColumn) SerializeSqlForColumnList(includeTableName bool, d Dialect, out *bytes.Buffer) error {
	return c.SerializeSql(d, out)
}

func (c *deferredLookupColumn) SerializeSql(d Dialect, out *bytes.Buffer) error {
	if c.cachedColumn != nil {
		return c.cachedColumn.SerializeSql(d, out)
	}

	col, err := c.table.getColumn(c.colName)
	if err != nil {
		return err
	}

	c.cachedColumn = col
	return col.SerializeSql(d, out)
}

func (c *deferredLookupColumn) setTableName(table string) error {
	return fmt.Errorf("lookup column '%s' should never have setTableName called on it", c.colName)
}
